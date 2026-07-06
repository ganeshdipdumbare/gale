package download_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ganeshdipdumbare/gale/internal/download"
)

func makeTestFile(size int) ([]byte, string) {
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}

func rangeServer(data []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			w.WriteHeader(http.StatusOK)
		case http.MethodGet:
			if rg := r.Header.Get("Range"); rg != "" {
				var start, end int
				_, _ = fmt.Sscanf(rg, "bytes=%d-%d", &start, &end)
				if end >= len(data) {
					end = len(data) - 1
				}
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(data[start : end+1])
				return
			}
			_, _ = w.Write(data)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
}

func TestDownloadResumeAfterInterrupt(t *testing.T) {
	data, hash := makeTestFile(3 * 1024 * 1024)
	srv := rangeServer(data)
	defer srv.Close()

	var requests atomic.Int32
	interruptAt := int32(2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		case http.MethodGet:
			n := requests.Add(1)
			if n == interruptAt {
				hj, ok := w.(http.Hijacker)
				if ok {
					conn, _, _ := hj.Hijack()
					conn.Close()
					return
				}
			}
			if rg := r.Header.Get("Range"); rg != "" {
				var start, end int
				_, _ = fmt.Sscanf(rg, "bytes=%d-%d", &start, &end)
				if end >= len(data) {
					end = len(data) - 1
				}
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(data)))
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(data[start : end+1])
				return
			}
			_, _ = w.Write(data)
		}
	}))
	defer server.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	dl := download.NewDownloader()
	spec := download.DownloadSpec{
		ID:         "test",
		URL:        server.URL,
		Dest:       dest,
		FileSHA256: hash,
		ChunkSize:  1024 * 1024,
		MaxWorkers: 2,
	}

	ctx1, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- dl.Download(ctx1, spec, nil)
	}()
	time.Sleep(300 * time.Millisecond)
	cancel()
	if err := <-errCh; err == nil {
		t.Fatal("expected cancel error")
	}

	ctx2 := context.Background()
	if err := dl.Download(ctx2, spec, nil); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if len(got) != len(data) {
		t.Fatalf("size %d want %d", len(got), len(data))
	}
}

func TestCorruptedChunkRefetched(t *testing.T) {
	data, hash := makeTestFile(2 * 1024 * 1024)
	srv := rangeServer(data)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	chunkSize := int64(1024 * 1024)
	spec := download.DownloadSpec{
		ID:         "test",
		URL:        srv.URL,
		Dest:       dest,
		FileSHA256: hash,
		ChunkSize:  chunkSize,
		MaxWorkers: 2,
	}

	// Seed a partial download with chunk 0 marked done but corrupted on disk.
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(dest, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteAt([]byte{0xFF}, 0)
	f.Close()

	m := download.NewManifest(spec.URL, nil, int64(len(data)), chunkSize, hash)
	m.Chunks[0].Done = true
	m.Chunks[0].SHA256 = "intentionally-wrong"
	if err := download.SaveManifest(dest, m); err != nil {
		t.Fatal(err)
	}

	dl := download.NewDownloader()
	if err := dl.Download(context.Background(), spec, nil); err != nil {
		t.Fatalf("refetch failed: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if !bytesEqual(got, data) {
		t.Fatal("content mismatch after refetch")
	}
}

func TestChecksumMismatchRejected(t *testing.T) {
	data, _ := makeTestFile(512 * 1024)
	srv := rangeServer(data)
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	dl := download.NewDownloader()
	spec := download.DownloadSpec{
		ID:         "test",
		URL:        srv.URL,
		Dest:       dest,
		FileSHA256: "deadbeef",
		ChunkSize:  256 * 1024,
		MaxWorkers: 2,
	}
	err := dl.Download(context.Background(), spec, nil)
	if err == nil {
		t.Fatal("expected checksum error")
	}
}

func TestMirrorFallback(t *testing.T) {
	data, hash := makeTestFile(256 * 1024)

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			return
		}
		http.Error(w, "fail", http.StatusInternalServerError)
	}))
	defer primary.Close()

	mirror := rangeServer(data)
	defer mirror.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	dl := download.NewDownloader()
	spec := download.DownloadSpec{
		ID:         "test",
		URL:        primary.URL,
		Mirrors:    []string{mirror.URL},
		Dest:       dest,
		FileSHA256: hash,
		ChunkSize:  128 * 1024,
		MaxWorkers: 2,
	}
	if err := dl.Download(context.Background(), spec, nil); err != nil {
		t.Fatalf("mirror fallback failed: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesEqual(got, data) {
		t.Fatal("content mismatch")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := download.NewManifest("http://example.com", []string{"http://mirror"}, 16, 8, "abc")
	if len(m.Chunks) != 2 {
		t.Fatalf("chunks %d", len(m.Chunks))
	}
	dest := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(dest, []byte{}, 0o644)
	if err := download.SaveManifest(dest, m); err != nil {
		t.Fatal(err)
	}
	loaded, err := download.LoadManifest(dest)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.TotalSize != 16 || loaded.ChunkSize != 8 {
		t.Fatalf("manifest mismatch %+v", loaded)
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSingleStreamFallback(t *testing.T) {
	data, hash := makeTestFile(1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
			return
		}
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "file.bin")
	dl := download.NewDownloader()
	spec := download.DownloadSpec{
		ID:         "test",
		URL:        srv.URL,
		Dest:       dest,
		FileSHA256: hash,
		MaxWorkers: 2,
	}
	if err := dl.Download(context.Background(), spec, nil); err != nil {
		t.Fatal(err)
	}
}
