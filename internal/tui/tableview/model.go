package tableview

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ganeshdipdumbare/gale/internal/db"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// Model lists installed packages.
type Model struct {
	table    table.Model
	quitting bool
}

func NewModel(packages []db.InstalledPackage) Model {
	columns := []table.Column{
		{Title: "Package", Width: 20},
		{Title: "Version", Width: 14},
		{Title: "Installed", Width: 20},
		{Title: "Size", Width: 10},
	}
	var rows []table.Row
	for _, p := range packages {
		rows = append(rows, table.Row{
			p.Name,
			p.Version,
			p.InstalledAt.Format(time.RFC3339),
			humanSize(p.Size),
		})
	}
	t := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(min(20, len(rows)+1)),
	)
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(lipgloss.Color("#5dade2"))
	t.SetStyles(s)
	return Model{table: t}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return theme.Title.Render("installed packages") + "\n\n" + m.table.View() + "\n" + theme.Muted.Render("q quit")
}

func humanSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	const u = 1024
	if n < u {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(u*u))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Plain renders installed packages for non-TUI mode.
func Plain(packages []db.InstalledPackage) string {
	if len(packages) == 0 {
		return "no packages installed\n"
	}
	var out string
	for _, p := range packages {
		out += fmt.Sprintf("%-20s %-12s %s\n", p.Name, p.Version, p.InstalledAt.Format("2006-01-02"))
	}
	return out
}
