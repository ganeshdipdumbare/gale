package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// DownloadSpec describes a file to download.
type DownloadSpec struct {
	ID         string
	URL        string
	Mirrors    []string
	Dest       string
	FileSHA256 string
	ChunkSize  int64
	MaxWorkers int
}

// Downloader performs resumable chunked parallel downloads.
type Downloader struct {
	Client *http.Client
	ghcr   *ghcrAuth
}

// copyBufPool recycles large buffers for chunk writes and verification.
var copyBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 1024*1024)
		return &b
	},
}

func (d *Downloader) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "gale/1.0 Homebrew-compat")
	req.Header.Set("Accept", "*/*")
}

type chunkJob struct {
	index  int
	offset int64
	size   int64
}

// Download fetches a file with resumable chunked parallel workers.
func (d *Downloader) Download(ctx context.Context, spec DownloadSpec, progress chan<- Event) error {
	if spec.ChunkSize <= 0 {
		spec.ChunkSize = DefaultChunkSize
	}
	if spec.MaxWorkers <= 0 {
		spec.MaxWorkers = runtime.NumCPU()
		if spec.MaxWorkers < 4 {
			spec.MaxWorkers = 4
		}
	}
	if err := EnsureParent(spec.Dest); err != nil {
		return err
	}

	emit := func(ev Event) {
		if progress == nil {
			return
		}
		ev.ID = spec.ID
		progress <- ev
	}

	emit(Event{State: StateDownloading})

	urls := append([]string{spec.URL}, spec.Mirrors...)
	urls = dedupe(urls)

	totalSize, supportsRange, err := d.probe(ctx, urls[0])
	if err != nil {
		emit(Event{State: StateFailed, Error: err})
		return err
	}

	if !supportsRange || totalSize <= spec.ChunkSize {
		return d.downloadSingle(ctx, spec, urls, totalSize, progress)
	}

	manifest, err := LoadManifest(spec.Dest)
	if err != nil {
		return err
	}
	if manifest == nil || manifest.TotalSize != totalSize || manifest.ChunkSize != spec.ChunkSize {
		manifest = NewManifest(spec.URL, spec.Mirrors, totalSize, spec.ChunkSize, spec.FileSHA256)
	} else {
		manifest.URL = spec.URL
		manifest.Mirrors = spec.Mirrors
		manifest.FileSHA256 = spec.FileSHA256
	}

	if err := d.prepareFile(spec.Dest, totalSize); err != nil {
		return err
	}
	if err := d.verifyExistingChunks(spec.Dest, manifest); err != nil {
		return err
	}
	if err := SaveManifest(spec.Dest, manifest); err != nil {
		return err
	}

	var bytesDone atomic.Int64
	bytesDone.Store(manifest.DoneBytes())

	var activeWorkers atomic.Int32
	workers := int32(spec.MaxWorkers)
	minWorkers := int32(1)
	maxWorkers := int32(spec.MaxWorkers * 2)
	if maxWorkers < 4 {
		maxWorkers = 4
	}

	start := time.Now()
	var progressMu sync.Mutex
	var lastEmit time.Time
	var lastBytes int64

	aimdTicker := time.NewTicker(2 * time.Second)
	defer aimdTicker.Stop()

	jobs := make(chan chunkJob, len(manifest.Chunks))
	for _, idx := range manifest.PendingChunks() {
		c := manifest.Chunks[idx]
		jobs <- chunkJob{index: c.Index, offset: c.Offset, size: c.Size}
	}
	close(jobs)

	var mu sync.Mutex
	var manifestDirty bool
	var lastManifestSave time.Time
	saveManifestLocked := func(force bool) error {
		if !manifestDirty && !force {
			return nil
		}
		if !force && time.Since(lastManifestSave) < 750*time.Millisecond {
			return nil
		}
		if err := SaveManifest(spec.Dest, manifest); err != nil {
			return err
		}
		manifestDirty = false
		lastManifestSave = time.Now()
		return nil
	}

	eg, egCtx := errgroup.WithContext(ctx)

	workerFn := func() {
		eg.Go(func() error {
			f, err := os.OpenFile(spec.Dest, os.O_RDWR, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()

			for job := range jobs {
				if err := egCtx.Err(); err != nil {
					return err
				}
				activeWorkers.Add(1)
				err := d.fetchChunkWithRetry(egCtx, f, urls, job)
				activeWorkers.Add(-1)
				if err != nil {
					return err
				}
				mu.Lock()
				manifest.Chunks[job.index].Done = true
				manifestDirty = true
				_ = saveManifestLocked(false)
				mu.Unlock()
				bytesDone.Add(job.size)

				progressMu.Lock()
				now := time.Now()
				if now.Sub(lastEmit) >= 100*time.Millisecond {
					done := bytesDone.Load()
					elapsed := now.Sub(start).Seconds()
					speed := float64(done) / elapsed
					remaining := totalSize - done
					var eta time.Duration
					if speed > 0 {
						eta = time.Duration(float64(remaining)/speed) * time.Second
					}
					emit(Event{
						State:         StateDownloading,
						BytesDone:     done,
						TotalBytes:    totalSize,
						Speed:         speed,
						ETA:           eta,
						ActiveWorkers: int(activeWorkers.Load()),
						ChunkIndex:    job.index,
					})
					lastEmit = now
					lastBytes = done
				}
				progressMu.Unlock()
			}
			return nil
		})
	}

	for i := int32(0); i < workers; i++ {
		workerFn()
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-aimdTicker.C:
				progressMu.Lock()
				done := bytesDone.Load()
				elapsed := time.Since(start).Seconds()
				if elapsed < 1 {
					progressMu.Unlock()
					continue
				}
				speed := float64(done) / elapsed
				recent := float64(done-lastBytes) / 2.0
				if recent > speed*1.1 && activeWorkers.Load() < maxWorkers {
					workers++
				} else if recent < speed*0.5 && workers > minWorkers {
					workers--
				}
				lastBytes = done
				progressMu.Unlock()
			}
		}
	}()

	if err := eg.Wait(); err != nil {
		if errors.Is(err, context.Canceled) {
			emit(Event{State: StatePaused, Error: err})
		} else {
			emit(Event{State: StateFailed, Error: err})
		}
		return err
	}
	mu.Lock()
	_ = saveManifestLocked(true)
	mu.Unlock()

	emit(Event{State: StateVerifying, BytesDone: totalSize, TotalBytes: totalSize})
	if err := d.verifyWholeFile(spec.Dest, spec.FileSHA256); err != nil {
		emit(Event{State: StateFailed, Error: err})
		return err
	}
	if err := DeleteManifest(spec.Dest); err != nil {
		return err
	}
	emit(Event{State: StateDone, BytesDone: totalSize, TotalBytes: totalSize})
	return nil
}

func (d *Downloader) probe(ctx context.Context, url string) (int64, bool, error) {
	// GHCR rejects HEAD; skip straight to ranged GET probe.
	if isGHCR(url) {
		return d.probeGet(ctx, url)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0, false, err
	}
	d.setHeaders(req)
	if err := d.authorize(ctx, req); err != nil {
		return 0, false, err
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, false, err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotFound {
		return d.probeGet(ctx, url)
	}
	if resp.StatusCode >= 400 {
		return 0, false, fmt.Errorf("HEAD %s: %s", url, resp.Status)
	}
	supports := resp.Header.Get("Accept-Ranges") == "bytes" || resp.StatusCode == http.StatusPartialContent
	return resp.ContentLength, supports, nil
}

func (d *Downloader) probeGet(ctx context.Context, url string) (int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, false, err
	}
	d.setHeaders(req)
	req.Header.Set("Range", "bytes=0-0")
	if err := d.authorize(ctx, req); err != nil {
		return 0, false, err
	}
	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, false, fmt.Errorf("GET probe %s: %s", url, resp.Status)
	}
	supports := resp.StatusCode == http.StatusPartialContent
	var total int64
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		var start, end int64
		_, _ = fmt.Sscanf(cr, "bytes %d-%d/%d", &start, &end, &total)
	}
	if total == 0 {
		total = resp.ContentLength
	}
	io.Copy(io.Discard, resp.Body)
	return total, supports, nil
}

func (d *Downloader) prepareFile(dest string, size int64) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return err
	}
	return nil
}

func (d *Downloader) verifyExistingChunks(dest string, m *PartManifest) error {
	f, err := os.Open(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	for i := range m.Chunks {
		c := &m.Chunks[i]
		if !c.Done {
			continue
		}
		h := sha256.New()
		remaining := c.Size
		offset := c.Offset
		for remaining > 0 {
			n := int64(len(buf))
			if n > remaining {
				n = remaining
			}
			_, err := f.ReadAt(buf[:n], offset)
			if err != nil {
				c.Done = false
				c.SHA256 = ""
				break
			}
			h.Write(buf[:n])
			offset += n
			remaining -= n
		}
		if c.Done {
			sum := hex.EncodeToString(h.Sum(nil))
			if c.SHA256 != "" && c.SHA256 != sum {
				c.Done = false
				c.SHA256 = ""
			} else {
				c.SHA256 = sum
			}
		}
	}
	return nil
}

func (d *Downloader) fetchChunkWithRetry(ctx context.Context, f *os.File, urls []string, job chunkJob) error {
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		url := urls[attempt%len(urls)]
		err := d.fetchChunk(ctx, f, url, job)
		if err == nil {
			return nil
		}
		lastErr = err
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff + jitter):
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
	return fmt.Errorf("chunk %d failed after retries: %w", job.index, lastErr)
}

func (d *Downloader) fetchChunk(ctx context.Context, f *os.File, url string, job chunkJob) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	end := job.offset + job.size - 1
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", job.offset, end))
	d.setHeaders(req)

	if err := d.authorize(ctx, req); err != nil {
		return err
	}

	resp, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET range %s: %s", url, resp.Status)
	}

	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)
	w := &writerAt{f: f, off: job.offset}
	written, err := io.CopyBuffer(w, resp.Body, *bufp)
	if err != nil {
		return err
	}
	if written != job.size {
		return fmt.Errorf("chunk %d: wrote %d, expected %d", job.index, written, job.size)
	}
	return nil
}

type writerAt struct {
	f   *os.File
	off int64
}

func (w *writerAt) Write(p []byte) (int, error) {
	n, err := w.f.WriteAt(p, w.off)
	w.off += int64(n)
	return n, err
}

func (d *Downloader) verifyWholeFile(path, expected string) error {
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	bufp := copyBufPool.Get().(*[]byte)
	defer copyBufPool.Put(bufp)
	if _, err := io.CopyBuffer(h, f, *bufp); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, expected)
	}
	return nil
}

func (d *Downloader) downloadSingle(ctx context.Context, spec DownloadSpec, urls []string, totalSize int64, progress chan<- Event) error {
	emit := func(ev Event) {
		if progress == nil {
			return
		}
		ev.ID = spec.ID
		progress <- ev
	}

	var lastErr error
	for i, url := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		d.setHeaders(req)
		if err := d.authorize(ctx, req); err != nil {
			return err
		}
		resp, err := d.Client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			lastErr = fmt.Errorf("GET %s: %s", url, resp.Status)
			if i < len(urls)-1 {
				continue
			}
			emit(Event{State: StateFailed, Error: lastErr})
			return lastErr
		}

		tmp := spec.Dest + ".tmp"
		f, err := os.Create(tmp)
		if err != nil {
			resp.Body.Close()
			return err
		}
		written, err := io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			os.Remove(tmp)
			lastErr = err
			continue
		}
		if totalSize > 0 && written != totalSize {
			os.Remove(tmp)
			lastErr = fmt.Errorf("size mismatch: got %d want %d", written, totalSize)
			continue
		}
		emit(Event{State: StateVerifying, BytesDone: written, TotalBytes: written})
		if err := d.verifyWholeFile(tmp, spec.FileSHA256); err != nil {
			os.Remove(tmp)
			emit(Event{State: StateFailed, Error: err})
			return err
		}
		if err := os.Rename(tmp, spec.Dest); err != nil {
			return err
		}
		emit(Event{State: StateDone, BytesDone: written, TotalBytes: written})
		return nil
	}
	if lastErr != nil {
		emit(Event{State: StateFailed, Error: lastErr})
	}
	return lastErr
}

func dedupe(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Pause is a no-op helper; cancellation of context pauses and preserves sidecar.
func Pause(ctx context.Context) context.CancelFunc {
	_, cancel := context.WithCancel(ctx)
	return cancel
}
