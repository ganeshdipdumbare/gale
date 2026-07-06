package index

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/vmihailenco/msgpack/v5"
)

const formulaAPI = "https://formulae.brew.sh/api/formula.json"

// Client fetches and caches the Homebrew formula index.
type Client struct {
	CacheDir string
	HTTP     *http.Client
}

func NewClient(cacheDir string) *Client {
	return &Client{
		CacheDir: cacheDir,
		HTTP:     &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *Client) cachePath() string {
	return filepath.Join(c.CacheDir, "index.mpack.zst")
}

func (c *Client) metaPath() string {
	return filepath.Join(c.CacheDir, "meta.json")
}

type meta struct {
	ETag      string    `json:"etag"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Load reads the local cached index.
func (c *Client) Load() (*Index, error) {
	data, err := os.ReadFile(c.cachePath())
	if err != nil {
		return nil, err
	}
	zr, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var idx Index
	dec := msgpack.NewDecoder(zr)
	if err := dec.Decode(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// Save writes the index to disk.
func (c *Client) Save(idx *Index) error {
	if err := os.MkdirAll(c.CacheDir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	if err := enc.Encode(idx); err != nil {
		return err
	}
	var zbuf bytes.Buffer
	zw, err := zstd.NewWriter(&zbuf)
	if err != nil {
		return err
	}
	if _, err := zw.Write(buf.Bytes()); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	tmp := c.cachePath() + ".tmp"
	if err := os.WriteFile(tmp, zbuf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.cachePath()); err != nil {
		return err
	}
	metaBytes, _ := json.Marshal(meta{ETag: idx.ETag, UpdatedAt: idx.UpdatedAt})
	return os.WriteFile(c.metaPath(), metaBytes, 0o644)
}

// Update fetches the Homebrew API and refreshes the local cache.
// This is a pragmatic approximation of binary diff: we diff in memory against
// the previous snapshot since Homebrew only exposes full JSON snapshots.
func (c *Client) Update(force bool) (*Index, int, error) {
	var etag string
	if b, err := os.ReadFile(c.metaPath()); err == nil {
		var m meta
		if json.Unmarshal(b, &m) == nil {
			etag = m.ETag
		}
	}

	req, err := http.NewRequest(http.MethodGet, formulaAPI, nil)
	if err != nil {
		return nil, 0, err
	}
	if etag != "" && !force {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		idx, err := c.Load()
		return idx, 0, err
	}
	if resp.StatusCode >= 400 {
		return nil, 0, fmt.Errorf("formula API: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	var raw []brewFormula
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, err
	}

	tag := platformTag()
	newIdx := &Index{
		UpdatedAt: time.Now().UTC(),
		ETag:      resp.Header.Get("ETag"),
		Packages:  make(map[string]Package, len(raw)),
	}
	for _, f := range raw {
		pkg := convertFormula(f, tag)
		if pkg.Name == "" {
			continue
		}
		newIdx.Packages[strings.ToLower(pkg.Name)] = pkg
	}

	changed := len(newIdx.Packages)
	if old, err := c.Load(); err == nil {
		changed = countChanges(old, newIdx)
	}
	if err := c.Save(newIdx); err != nil {
		return nil, 0, err
	}
	return newIdx, changed, nil
}

func countChanges(old, new *Index) int {
	n := 0
	for name, pkg := range new.Packages {
		prev, ok := old.Packages[name]
		if !ok || prev.Version != pkg.Version || prev.Bottle.SHA256 != pkg.Bottle.SHA256 {
			n++
		}
	}
	return n
}

type brewFormula struct {
	Name         string `json:"name"`
	Desc         string `json:"desc"`
	Homepage     string `json:"homepage"`
	Dependencies []string `json:"dependencies"`
	Versions     struct {
		Stable string `json:"stable"`
	} `json:"versions"`
	Bottle struct {
		Stable struct {
			RootURL string `json:"root_url"`
			Files   map[string]struct {
				URL    string `json:"url"`
				SHA256 string `json:"sha256"`
				Cellar string `json:"cellar"`
			} `json:"files"`
		} `json:"stable"`
	} `json:"bottle"`
}

func convertFormula(f brewFormula, tag string) Package {
	file, ok := f.Bottle.Stable.Files[tag]
	if !ok {
		for _, t := range fallbackTags(tag) {
			if file, ok = f.Bottle.Stable.Files[t]; ok {
				tag = t
				break
			}
		}
	}
	var bottle Bottle
	if ok && file.SHA256 != "" {
		url := file.URL
		if url == "" && f.Bottle.Stable.RootURL != "" {
			url = fmt.Sprintf("%s/%s/blobs/sha256:%s", f.Bottle.Stable.RootURL, f.Name, file.SHA256)
		}
		bottle = Bottle{URL: url, SHA256: file.SHA256, Tag: tag}
	}
	if f.Name == "" {
		return Package{}
	}
	return Package{
		Name:         f.Name,
		Version:      f.Versions.Stable,
		Description:  f.Desc,
		Dependencies: f.Dependencies,
		Homepage:     f.Homepage,
		Bottle:       bottle,
	}
}

func platformTag() string {
	if runtime.GOOS == "darwin" {
		if runtime.GOARCH == "arm64" {
			// Prefer newest known tags; caller fallbacks handle older macOS.
			return "arm64_tahoe"
		}
		return "tahoe"
	}
	if runtime.GOARCH == "arm64" {
		return "arm64_linux"
	}
	return "x86_64_linux"
}

func fallbackTags(primary string) []string {
	switch primary {
	case "arm64_tahoe":
		return []string{"arm64_sequoia", "arm64_sonoma", "arm64_ventura"}
	case "tahoe":
		return []string{"sequoia", "sonoma", "ventura"}
	default:
		return nil
	}
}

// Get returns a package by name.
func (idx *Index) Get(name string) (Package, bool) {
	p, ok := idx.Packages[strings.ToLower(name)]
	return p, ok
}
