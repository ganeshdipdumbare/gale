// Package detail renders the `gale info <pkg>` screen with full package
// metadata, scrolling when it doesn't fit the terminal.
package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
	"github.com/mattn/go-runewidth"
)

const (
	headerLines = 2
	footerLines = 2
)

type Model struct {
	pkg      index.Package
	width    int
	height   int
	quitting bool
	ready    bool
	vp       viewport.Model
}

func NewModel(pkg index.Package) Model {
	return Model{pkg: pkg}
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
			m.vp.SetContent(m.renderBody())
			m.ready = true
		} else {
			m.vp.Width = msg.Width
			m.vp.Height = vpHeight
			m.vp.SetContent(m.renderBody())
		}
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
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

func (m Model) renderBody() string {
	p := m.pkg
	width := m.width
	if width <= 0 {
		width = 80
	}
	var b strings.Builder

	if p.Description != "" {
		b.WriteString(theme.Text.Render(wrap(p.Description, width)))
		b.WriteString("\n\n")
	}

	if p.Homepage != "" {
		b.WriteString(theme.Muted.Render("Homepage  "))
		b.WriteString(theme.AccentS.Render(p.Homepage))
		b.WriteString("\n")
	}
	if p.Bottle.SHA256 != "" {
		b.WriteString(theme.Muted.Render("SHA256    "))
		b.WriteString(theme.FaintS.Render(p.Bottle.SHA256))
		b.WriteString("\n")
	}
	if p.Bottle.Size > 0 {
		b.WriteString(theme.Muted.Render("Size      "))
		b.WriteString(theme.Text.Render(theme.HumanBytes(p.Bottle.Size)))
		b.WriteString("\n")
	}

	if len(p.Dependencies) > 0 {
		b.WriteString("\n")
		b.WriteString(theme.Muted.Render(fmt.Sprintf("Depends on (%d)", len(p.Dependencies))))
		b.WriteString("\n")
		for _, d := range p.Dependencies {
			b.WriteString("  ")
			b.WriteString(theme.Text.Render(d))
			b.WriteString("\n")
		}
	}

	if b.Len() == 0 {
		b.WriteString(theme.Muted.Render("no additional metadata available"))
	}

	return strings.TrimRight(b.String(), "\n")
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	p := m.pkg
	var b strings.Builder
	b.WriteString(theme.Screen(p.Name, p.Version))
	b.WriteString("\n\n")

	if !m.ready {
		b.WriteString(m.renderBody())
		b.WriteString("\n\n")
		b.WriteString(theme.Help("q", "close"))
		return b.String()
	}

	b.WriteString(m.vp.View())
	b.WriteString("\n")
	hint := theme.ScrollHint(!m.vp.AtTop(), !m.vp.AtBottom(), m.width)
	if hint != "" {
		b.WriteString(hint)
		b.WriteString("\n")
	}
	b.WriteString(theme.Help("↑↓", "scroll", "q", "close"))
	return b.String()
}

// wrap performs display-width-aware word wrapping (safe for wide/CJK runes)
// instead of hard byte slicing, which used to corrupt multi-byte characters
// mid-codepoint and ignore terminal display width.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return s
	}
	var lines []string
	var line strings.Builder
	lineW := 0
	for _, word := range words {
		wW := runewidth.StringWidth(word)
		if lineW > 0 && lineW+1+wW > width {
			lines = append(lines, line.String())
			line.Reset()
			lineW = 0
		}
		if lineW > 0 {
			line.WriteByte(' ')
			lineW++
		}
		line.WriteString(word)
		lineW += wW
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

func Plain(pkg index.Package) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", pkg.Name, pkg.Version)
	if pkg.Description != "" {
		b.WriteString(pkg.Description + "\n")
	}
	if pkg.Homepage != "" {
		b.WriteString("homepage: " + pkg.Homepage + "\n")
	}
	if len(pkg.Dependencies) > 0 {
		b.WriteString("dependencies: " + strings.Join(pkg.Dependencies, ", ") + "\n")
	}
	return b.String()
}
