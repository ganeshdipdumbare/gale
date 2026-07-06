package tree

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// Model shows the dependency tree before install confirmation.
type Model struct {
	lines    []string
	confirmed bool
	quitting bool
	width    int
}

func NewModel(g *resolver.Graph, root string) Model {
	var lines []string
	if g != nil {
		lines = g.TreeLines(root)
	}
	return Model{lines: lines}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "enter":
			m.confirmed = true
			return m, tea.Quit
		case "n", "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Title.Render("dependency tree"))
	b.WriteString("\n\n")
	for _, line := range m.lines {
		b.WriteString(theme.Accent.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(theme.Subtitle.Render("y/enter confirm  n cancel"))
	return b.String()
}

func (m Model) Confirmed() bool { return m.confirmed && !m.quitting }
