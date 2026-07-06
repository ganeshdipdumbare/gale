package index

import (
	"strings"

	fuzzy "github.com/sahilm/fuzzy"
)

// Search performs fuzzy search over package names and descriptions.
func (idx *Index) Search(query string, limit int) []SearchResult {
	if idx == nil || len(idx.Packages) == 0 {
		return nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}

	names := make([]string, 0, len(idx.Packages))
	lookup := make(map[string]Package, len(idx.Packages))
	for name, pkg := range idx.Packages {
		names = append(names, name)
		lookup[name] = pkg
	}

	matches := fuzzy.Find(query, names)
	out := make([]SearchResult, 0, min(limit, len(matches)))
	for _, m := range matches {
		if len(out) >= limit {
			break
		}
		pkg := lookup[m.Str]
		out = append(out, SearchResult{
			Package:    pkg,
			Score:      m.Score,
			MatchRange: m.MatchedIndexes,
		})
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
