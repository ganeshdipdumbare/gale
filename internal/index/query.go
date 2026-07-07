package index

import (
	"sort"
	"strings"

	fuzzy "github.com/sahilm/fuzzy"
)

// Search performs fuzzy search over package names, ranked so the closest
// match to the query always sorts first.
//
// fuzzy.Find alone ranks by subsequence-match heuristics (adjacency,
// separators, camelCase) which can rank a longer, loosely-related name above
// an exact or prefix match — e.g. querying "py" can rank "python-yq" above
// "python" because the "y" after the "-" separator earns a bonus. To keep
// results intuitive, matches are re-ranked by closeness to the literal query
// (exact name > prefix > substring > fuzzy subsequence) before falling back
// to fuzzy's own score to break ties within each tier.
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
	rankByCloseness(query, matches)

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

// rankByCloseness stable-sorts fuzzy matches into closeness tiers (exact,
// prefix, substring, then plain fuzzy subsequence), using fuzzy's own score
// to order within a tier. This guarantees the literally-closest name to the
// query always appears first, regardless of fuzzy's internal bonus quirks.
func rankByCloseness(query string, matches fuzzy.Matches) {
	q := strings.ToLower(strings.TrimSpace(query))
	sort.SliceStable(matches, func(i, j int) bool {
		ti, tj := closenessTier(q, matches[i].Str), closenessTier(q, matches[j].Str)
		if ti != tj {
			return ti > tj
		}
		// Within an exact/prefix/substring tier, a shorter name is a more
		// precise (closer) match to the query than a longer name that
		// merely happens to contain it (e.g. "python" over "python-yq" for
		// query "py") — length is a much more reliable closeness signal
		// here than fuzzy's adjacency/separator bonuses.
		if ti > 0 {
			li, lj := len(matches[i].Str), len(matches[j].Str)
			if li != lj {
				return li < lj
			}
		}
		return matches[i].Score > matches[j].Score
	})
}

// closenessTier scores how literally close name is to query: higher is
// closer. Ties within a tier fall back to the fuzzy score.
func closenessTier(query, name string) int {
	n := strings.ToLower(name)
	switch {
	case n == query:
		return 3
	case strings.HasPrefix(n, query):
		return 2
	case strings.Contains(n, query):
		return 1
	default:
		return 0
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
