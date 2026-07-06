package detail

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// Model shows package detail information.
type Model struct {
	pkg      index.Package
	width    int
	quitting bool
}

func NewModel(pkg index.Package) Model {
	return Model{pkg: pkg}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	p := m.pkg
	var b strings.Builder
	b.WriteString(theme.Title.Render(p.Name))
	b.WriteString(" ")
	b.WriteString(theme.Subtitle.Render(p.Version))
	b.WriteString("\n\n")
	if p.Description != "" {
		b.WriteString(p.Description)
		b.WriteString("\n\n")
	}
	if p.Homepage != "" {
		b.WriteString(theme.Muted.Render("homepage: "))
		b.WriteString(p.Homepage)
		b.WriteString("\n")
	}
	if p.Bottle.SHA256 != "" {
		b.WriteString(theme.Muted.Render("bottle: "))
		b.WriteString(p.Bottle.SHA256[:16])
		b.WriteString("…\n")
	}
	if len(p.Dependencies) > 0 {
		b.WriteString("\n")
		b.WriteString(theme.Accent.Render("dependencies:"))
		b.WriteString("\n")
		for _, d := range p.Dependencies {
			b.WriteString("  • ")
			b.WriteString(d)
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(theme.Muted.Render("q quit"))
	return b.String()
}

// Plain renders package info as plain text for non-TUI mode.
func Plain(pkg index.Package) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", pkg.Name, pkg.Version)
	if pkg.Description != "" {
		b.WriteString(pkg.Description + "\n")
	}
	if len(pkg.Dependencies) > 0 {
		b.WriteString("dependencies: " + strings.Join(pkg.Dependencies, ", ") + "\n")
	}
	return b.String()
}
