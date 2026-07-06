package download

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultChunkSize = 8 * 1024 * 1024 // 8MB
	PartSuffix       = ".gale-part.json"
)

// ChunkState tracks one chunk of a partial download.
type ChunkState struct {
	Index  int    `json:"index"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256,omitempty"`
	Done   bool   `json:"done"`
}

// PartManifest is the sidecar persisted next to a partial download.
type PartManifest struct {
	URL        string       `json:"url"`
	Mirrors    []string     `json:"mirrors,omitempty"`
	TotalSize  int64        `json:"total_size"`
	ChunkSize  int64        `json:"chunk_size"`
	FileSHA256 string       `json:"file_sha256"`
	Chunks     []ChunkState `json:"chunks"`
	UpdatedAt  time.Time    `json:"updated_at"`
}

func partPath(dest string) string {
	return dest + PartSuffix
}

// LoadManifest reads an existing sidecar or returns nil if absent.
func LoadManifest(dest string) (*PartManifest, error) {
	data, err := os.ReadFile(partPath(dest))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var m PartManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest atomically writes the sidecar.
func SaveManifest(dest string, m *PartManifest) error {
	m.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := partPath(dest) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, partPath(dest))
}

// DeleteManifest removes the sidecar.
func DeleteManifest(dest string) error {
	err := os.Remove(partPath(dest))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// NewManifest builds a fresh manifest for a file of totalSize.
func NewManifest(url string, mirrors []string, totalSize, chunkSize int64, fileSHA256 string) *PartManifest {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	n := int((totalSize + chunkSize - 1) / chunkSize)
	chunks := make([]ChunkState, n)
	for i := 0; i < n; i++ {
		offset := int64(i) * chunkSize
		size := chunkSize
		if offset+size > totalSize {
			size = totalSize - offset
		}
		chunks[i] = ChunkState{
			Index:  i,
			Offset: offset,
			Size:   size,
			Done:   false,
		}
	}
	return &PartManifest{
		URL:        url,
		Mirrors:    mirrors,
		TotalSize:  totalSize,
		ChunkSize:  chunkSize,
		FileSHA256: fileSHA256,
		Chunks:     chunks,
		UpdatedAt:  time.Now().UTC(),
	}
}

// DoneBytes returns bytes completed according to manifest.
func (m *PartManifest) DoneBytes() int64 {
	var n int64
	for _, c := range m.Chunks {
		if c.Done {
			n += c.Size
		}
	}
	return n
}

// PendingChunks returns indices of chunks that still need fetching.
func (m *PartManifest) PendingChunks() []int {
	var out []int
	for _, c := range m.Chunks {
		if !c.Done {
			out = append(out, c.Index)
		}
	}
	return out
}

// EnsureParent creates the parent directory for dest.
func EnsureParent(dest string) error {
	return os.MkdirAll(filepath.Dir(dest), 0o755)
}
