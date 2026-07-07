package resolver

import (
	"strings"
	"testing"

	"github.com/ganeshdipdumbare/gale/internal/index"
)

func newTestIndex(pkgs map[string][]string) *index.Index {
	idx := &index.Index{Packages: make(map[string]index.Package, len(pkgs))}
	for name, deps := range pkgs {
		idx.Packages[name] = index.Package{
			Name:         name,
			Version:      "1.0",
			Dependencies: deps,
			Bottle:       index.Bottle{URL: "https://example.com/" + name, SHA256: "sha-" + name},
		}
	}
	return idx
}

func TestMultiTreeLinesSingleRoot(t *testing.T) {
	idx := newTestIndex(map[string][]string{
		"a": {"b", "c"},
		"b": nil,
		"c": nil,
	})
	g, err := BuildGraph(idx, "a")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	lines := g.MultiTreeLines([]string{"a"})
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "a ") {
		t.Fatalf("expected first line to be root 'a', got %v", lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "b 1.0") || !strings.Contains(joined, "c 1.0") {
		t.Errorf("expected both deps in tree output, got:\n%s", joined)
	}
}

func TestMultiTreeLinesMultipleRootsSharedDep(t *testing.T) {
	// a and b both depend on shared; shared should render fully once and be
	// marked "already shown above" the second time so diamond graphs don't
	// blow up the tree size.
	idx := newTestIndex(map[string][]string{
		"a":      {"shared"},
		"b":      {"shared"},
		"shared": nil,
	})
	g, err := BuildGraph(idx, "a", "b")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	joined := strings.Join(g.MultiTreeLines([]string{"a", "b"}), "\n")

	full, marked := 0, 0
	for _, line := range strings.Split(joined, "\n") {
		if !strings.Contains(line, "shared 1.0") {
			continue
		}
		if strings.Contains(line, "already shown above") {
			marked++
		} else {
			full++
		}
	}
	if full != 1 {
		t.Errorf("expected exactly one fully-expanded 'shared' node, got %d in:\n%s", full, joined)
	}
	if marked != 1 {
		t.Errorf("expected exactly one 'already shown above' marker, got %d in:\n%s", marked, joined)
	}
	if !strings.Contains(joined, "a 1.0") || !strings.Contains(joined, "b 1.0") {
		t.Errorf("expected both requested roots present, got:\n%s", joined)
	}
}

func TestMultiTreeLinesDuplicateRootIgnored(t *testing.T) {
	idx := newTestIndex(map[string][]string{"a": nil})
	g, err := BuildGraph(idx, "a")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	lines := g.MultiTreeLines([]string{"a", "a"})
	count := strings.Count(strings.Join(lines, "\n"), "a 1.0")
	if count != 1 {
		t.Errorf("requesting the same root twice should render once, got %d occurrences", count)
	}
}

func TestMultiTreeLinesUnknownRoot(t *testing.T) {
	g := &Graph{Nodes: map[string]Node{}}
	lines := g.MultiTreeLines([]string{"missing"})
	if len(lines) != 1 || !strings.Contains(lines[0], "missing") {
		t.Errorf("expected a single line describing the missing root, got %v", lines)
	}
}

func TestMultiTreeLinesEmptyGraph(t *testing.T) {
	g := &Graph{Nodes: map[string]Node{}}
	lines := g.MultiTreeLines(nil)
	if len(lines) != 1 {
		t.Errorf("expected a single placeholder line for an empty graph, got %v", lines)
	}
}

func TestTreeLinesBackCompat(t *testing.T) {
	idx := newTestIndex(map[string][]string{"a": {"b"}, "b": nil})
	g, err := BuildGraph(idx, "a")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	if got := g.TreeLines("a"); len(got) < 2 {
		t.Errorf("TreeLines should still work as a single-root convenience wrapper, got %v", got)
	}
}
