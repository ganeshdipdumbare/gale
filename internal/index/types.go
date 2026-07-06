package index

import (
	"time"
)

// Package is the normalized index entry.
type Package struct {
	Name         string   `msgpack:"name"`
	Version      string   `msgpack:"version"`
	Description  string   `msgpack:"description"`
	Dependencies []string `msgpack:"dependencies"`
	Bottle       Bottle   `msgpack:"bottle"`
	Homepage     string   `msgpack:"homepage,omitempty"`
}

// HasBottle reports whether the package has a downloadable bottle for this platform.
func (p Package) HasBottle() bool {
	return p.Bottle.URL != "" && p.Bottle.SHA256 != ""
}

// Bottle describes a prebuilt binary artifact.
type Bottle struct {
	URL    string `msgpack:"url"`
	SHA256 string `msgpack:"sha256"`
	Size   int64  `msgpack:"size"`
	Tag    string `msgpack:"tag"`
}

// Index is the local package index.
type Index struct {
	UpdatedAt time.Time          `msgpack:"updated_at"`
	ETag      string             `msgpack:"etag"`
	Packages  map[string]Package `msgpack:"packages"`
}

// SearchResult is a fuzzy search hit.
type SearchResult struct {
	Package    Package
	Score      int
	MatchRange []int
}
