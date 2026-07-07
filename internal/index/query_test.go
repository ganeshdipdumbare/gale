package index

import "testing"

func newSearchIndex(names ...string) *Index {
	idx := &Index{Packages: make(map[string]Package, len(names))}
	for _, n := range names {
		idx.Packages[n] = Package{Name: n, Version: "1.0"}
	}
	return idx
}

func firstName(results []SearchResult) string {
	if len(results) == 0 {
		return ""
	}
	return results[0].Package.Name
}

func TestSearchExactMatchWins(t *testing.T) {
	idx := newSearchIndex("python", "python-yq", "pythonpy", "ipython", "micropython")
	got := firstName(idx.Search("python", 10))
	if got != "python" {
		t.Errorf("exact name match should always be first, got %q", got)
	}
}

func TestSearchPrefixBeatsLongerCompoundName(t *testing.T) {
	// Regression test: fuzzy's separator-bonus heuristic used to rank
	// "python-yq" above "python" for query "py" because the "y" following
	// the "-" earned a bonus. The literal prefix match must win.
	idx := newSearchIndex("python", "python-yq", "pythonpy", "ipython", "micropython")
	got := firstName(idx.Search("py", 10))
	if got != "python" {
		t.Errorf("shortest prefix match should be first for query 'py', got %q", got)
	}
}

func TestSearchPrefixTierOrdersByLength(t *testing.T) {
	idx := newSearchIndex("node", "nodejs", "node-fetch")
	results := idx.Search("nod", 10)
	var order []string
	for _, r := range results {
		order = append(order, r.Package.Name)
	}
	want := []string{"node", "nodejs", "node-fetch"}
	if len(order) != len(want) {
		t.Fatalf("got %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("position %d: got %q, want %q (full order %v)", i, order[i], want[i], order)
		}
	}
}

func TestSearchExactTierBeatsPrefixTier(t *testing.T) {
	idx := newSearchIndex("node", "nodejs", "node-fetch")
	got := firstName(idx.Search("node", 10))
	if got != "node" {
		t.Errorf("exact match 'node' should beat prefix matches, got %q", got)
	}
}

func TestSearchSubstringTierOrdersByLength(t *testing.T) {
	idx := newSearchIndex("python", "jython", "ipython", "python-yq")
	results := idx.Search("thon", 10)
	if len(results) == 0 {
		t.Fatal("expected substring matches for 'thon'")
	}
	// python/jython (6 chars) contain "thon" as a substring and should sort
	// ahead of the 7+ char names.
	top := results[0].Package.Name
	if top != "python" && top != "jython" {
		t.Errorf("expected one of the shortest substring matches first, got %q (full: %v)", top, namesOf(results))
	}
}

func TestSearchEmptyQueryReturnsNil(t *testing.T) {
	idx := newSearchIndex("python")
	if got := idx.Search("   ", 10); got != nil {
		t.Errorf("blank query should return nil, got %v", got)
	}
}

func TestSearchNilIndexDoesNotPanic(t *testing.T) {
	var idx *Index
	if got := idx.Search("anything", 10); got != nil {
		t.Errorf("nil index should return nil, got %v", got)
	}
}

func TestSearchRespectsLimit(t *testing.T) {
	idx := newSearchIndex("node", "nodejs", "node-fetch", "noodle", "nodemon")
	got := idx.Search("no", 2)
	if len(got) != 2 {
		t.Errorf("expected exactly 2 results respecting the limit, got %d", len(got))
	}
}

func TestSearchNoMatchesReturnsEmpty(t *testing.T) {
	idx := newSearchIndex("python", "node")
	got := idx.Search("zzz-nonexistent-zzz", 10)
	if len(got) != 0 {
		t.Errorf("expected no matches, got %v", namesOf(got))
	}
}

func namesOf(results []SearchResult) []string {
	out := make([]string, len(results))
	for i, r := range results {
		out[i] = r.Package.Name
	}
	return out
}
