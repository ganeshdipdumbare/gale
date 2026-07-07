package tree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
)

func newTestGraph(t *testing.T, lineCount int) *resolver.Graph {
	t.Helper()
	idx := &index.Index{Packages: map[string]index.Package{}}
	var deps []string
	for i := 0; i < lineCount; i++ {
		name := "dep" + string(rune('a'+i))
		deps = append(deps, name)
		idx.Packages[name] = index.Package{
			Name:    name,
			Version: "1.0",
			Bottle:  index.Bottle{URL: "https://x", SHA256: "s" + name},
		}
	}
	idx.Packages["root"] = index.Package{
		Name:         "root",
		Version:      "1.0",
		Dependencies: deps,
		Bottle:       index.Bottle{URL: "https://x", SHA256: "sroot"},
	}
	g, err := resolver.BuildGraph(idx, "root")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	return g
}

func TestTreeModelScrollsWithoutOverflow(t *testing.T) {
	g := newTestGraph(t, 50) // large tree, forces scrolling on a small terminal
	m := NewModel(g, "root")

	// Small terminal: only a handful of rows fit.
	res, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	m = res.(Model)

	if !m.ready {
		t.Fatal("model should be ready after receiving a WindowSizeMsg")
	}
	if m.vp.Height >= len(m.lines) {
		t.Fatalf("viewport height (%d) should be smaller than content (%d lines) to force scrolling", m.vp.Height, len(m.lines))
	}

	view := m.View()
	if strings.Count(view, "\n") > 10+2 { // small slack for ANSI/help lines
		t.Errorf("rendered view has more lines than the terminal height allows: %d lines\n%s", strings.Count(view, "\n"), view)
	}
	if !m.vp.AtTop() {
		t.Errorf("viewport should start at the top")
	}

	// Scroll down and confirm the offset actually moves (this is the bug we're fixing).
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := res.(Model)
	if m2.vp.YOffset == m.vp.YOffset {
		t.Errorf("expected scroll offset to change after pressing down, stayed at %d", m.vp.YOffset)
	}

	// PgDown should move further and not panic even near the bottom.
	for i := 0; i < 20; i++ {
		res, _ = m2.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		m2 = res.(Model)
	}
	if !m2.vp.AtBottom() {
		t.Errorf("expected repeated page-down to reach the bottom of a bounded viewport")
	}
}

func TestTreeModelConfirmAndCancel(t *testing.T) {
	g := newTestGraph(t, 2)
	m := NewModel(g, "root")

	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	mm := res.(Model)
	if !mm.Confirmed() {
		t.Errorf("'y' should confirm the install")
	}
	if cmd == nil {
		t.Errorf("expected a quit command after confirming")
	}

	m = NewModel(g, "root")
	res, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	mm = res.(Model)
	if mm.Confirmed() {
		t.Errorf("'n' should not confirm the install")
	}
}

func TestTreeModelMultiRoot(t *testing.T) {
	idx := &index.Index{Packages: map[string]index.Package{
		"a": {Name: "a", Version: "1.0", Bottle: index.Bottle{URL: "u", SHA256: "sa"}},
		"b": {Name: "b", Version: "1.0", Bottle: index.Bottle{URL: "u", SHA256: "sb"}},
	}}
	g, err := resolver.BuildGraph(idx, "a", "b")
	if err != nil {
		t.Fatalf("BuildGraph: %v", err)
	}
	m := NewModel(g, "a", "b")
	joined := strings.Join(m.lines, "\n")
	if !strings.Contains(joined, "a 1.0") || !strings.Contains(joined, "b 1.0") {
		t.Errorf("expected both requested roots in tree lines, got: %v", m.lines)
	}
}

func TestTreeModelEmptyGraph(t *testing.T) {
	m := NewModel(nil, "root")
	res, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	mm := res.(Model)
	view := mm.View()
	if view == "" {
		t.Error("expected non-empty view even for a nil graph")
	}
}
