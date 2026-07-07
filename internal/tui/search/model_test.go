package search

import (
	"strings"
	"testing"

	"github.com/ganeshdipdumbare/gale/internal/index"
)

func newTestIndex(names ...string) *index.Index {
	idx := &index.Index{Packages: make(map[string]index.Package, len(names))}
	for _, n := range names {
		idx.Packages[n] = index.Package{Name: n, Version: "1.0", Description: "desc " + n}
	}
	return idx
}

func TestNewModelEmptyResultsShowsEmptyState(t *testing.T) {
	idx := newTestIndex("foo", "bar")
	m := NewModel(idx, "nonexistent-query-xyz")
	if !m.empty {
		t.Fatal("expected empty flag to be set when no results match")
	}
	view := m.View()
	if !strings.Contains(view, "no packages found") {
		t.Errorf("expected empty-state message, got:\n%s", view)
	}
}

func TestNewModelNilIndexIsEmpty(t *testing.T) {
	m := NewModel(nil, "anything")
	if !m.empty {
		t.Error("a nil index should be treated as zero results, not panic")
	}
}

func TestNewModelWithResultsIsNotEmpty(t *testing.T) {
	idx := newTestIndex("foo", "bar")
	m := NewModel(idx, "foo")
	if m.empty {
		t.Error("expected a match for 'foo' to not be flagged empty")
	}
}
