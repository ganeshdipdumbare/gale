// Package tree renders the dependency confirmation screen shown before
// `gale install` downloads anything.
package tree

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// headerLines/footerLines reserve space around the scrollable viewport for
// the title and help hint so the confirm/cancel prompt is never pushed
// off-screen by a large tree.
const (
	headerLines = 2
	footerLines = 3
)

type Model struct {
	lines     []string
	toInstall int
	confirmed bool
	quitting  bool
	width     int
	height    int
	ready     bool
	vp        viewport.Model
}

// NewModel builds the confirmation tree for one or more requested root
// packages. Passing every requested root (not just the first) ensures
// multi-package installs (`gale install a b c`) show the full plan.
func NewModel(g *resolver.Graph, roots ...string) Model {
	var lines []string
	toInstall := 0
	if g != nil {
		lines = g.MultiTreeLines(roots)
		toInstall = len(g.Order)
	}
	return Model{lines: lines, toInstall: toInstall}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpHeight := msg.Height - headerLines - footerLines
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.vp = viewport.New(msg.Width, vpHeight)
			m.vp.SetContent(m.renderedContent())
			m.ready = true
		} else {
			m.vp.Width = msg.Width
			m.vp.Height = vpHeight
			m.vp.SetContent(m.renderedContent())
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			return m, tea.Quit
		case "n", "N", "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			// Only confirm on enter; keep it distinct from navigation keys
			// below so arrow/page keys always scroll instead of accidentally
			// being swallowed.
			m.confirmed = true
			return m, tea.Quit
		}
	}
	if m.ready {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) renderedContent() string {
	if len(m.lines) == 0 {
		return theme.Muted.Render("  nothing to install")
	}
	width := m.width
	var b strings.Builder
	for i, line := range m.lines {
		if line == "" {
			b.WriteString("\n")
			continue
		}
		display := line
		if width > 2 {
			display = theme.Truncate(line, width)
		}
		if i == 0 {
			b.WriteString(theme.Bold.Render(display))
		} else if strings.HasSuffix(line, "(already shown above)") {
			b.WriteString(theme.FaintS.Render(display))
		} else {
			b.WriteString(theme.Muted.Render(display))
		}
		if i != len(m.lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func (m Model) View() string {
	if m.quitting {
		return theme.Muted.Render("Cancelled.") + "\n"
	}
	if !m.ready {
		return theme.Screen("Install", "dependencies") + "\n\n" + theme.Muted.Render("loading…")
	}

	var b strings.Builder
	ctx := fmt.Sprintf("%d package", m.toInstall)
	if m.toInstall != 1 {
		ctx += "s"
	}
	b.WriteString(theme.Screen("Install", ctx+" to download"))
	b.WriteString("\n\n")
	b.WriteString(m.vp.View())
	b.WriteString("\n")

	hint := theme.ScrollHint(!m.vp.AtTop(), !m.vp.AtBottom(), m.width)
	if hint != "" {
		b.WriteString(hint)
		b.WriteString("\n")
	}
	b.WriteString(theme.Help("y", "proceed", "n", "cancel", "↑↓", "scroll"))
	return b.String()
}

// Confirmed reports whether the user accepted the install plan.
func (m Model) Confirmed() bool { return m.confirmed && !m.quitting }
