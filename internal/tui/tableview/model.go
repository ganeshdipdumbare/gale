// Package tableview renders the `gale list` screen: a scrollable table of
// installed packages.
package tableview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/db"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// Fixed-width columns; the Package column absorbs any extra/short space.
const (
	colVersion   = 14
	colInstalled = 12
	colSize      = 10
	minNameCol   = 10
)

type Model struct {
	table    table.Model
	packages []db.InstalledPackage
	quitting bool
	width    int
	height   int
}

func NewModel(packages []db.InstalledPackage) Model {
	t := table.New(
		table.WithColumns(columnsForWidth(0)),
		table.WithRows(rowsFor(packages)),
		table.WithFocused(true),
		table.WithHeight(tableHeight(0, len(packages))),
	)
	applyStyles(&t)
	return Model{table: t, packages: packages}
}

func applyStyles(t *table.Model) {
	s := table.DefaultStyles()
	s.Header = s.Header.Foreground(theme.Dim).BorderForeground(theme.Border).Bold(false)
	s.Cell = s.Cell.Foreground(theme.Fg)
	s.Selected = s.Selected.Foreground(theme.Fg).Background(theme.Sel).Bold(true)
	t.SetStyles(s)
}

func columnsForWidth(width int) []table.Column {
	nameW := minNameCol
	if width > 0 {
		// Reserve space for the other three columns plus inter-column gaps
		// added by the table component.
		reserved := colVersion + colInstalled + colSize + 8
		if w := width - reserved; w > minNameCol {
			nameW = w
		}
	} else {
		nameW = 22
	}
	return []table.Column{
		{Title: "Package", Width: nameW},
		{Title: "Version", Width: colVersion},
		{Title: "Installed", Width: colInstalled},
		{Title: "Size", Width: colSize},
	}
}

func rowsFor(packages []db.InstalledPackage) []table.Row {
	rows := make([]table.Row, 0, len(packages))
	for _, p := range packages {
		rows = append(rows, table.Row{
			p.Name,
			p.Version,
			p.InstalledAt.Format("2006-01-02"),
			humanSize(p.Size),
		})
	}
	return rows
}

// tableHeight reserves room for the title, help line, and table header so
// the whole screen fits within the terminal instead of overflowing it.
func tableHeight(termHeight, rowCount int) int {
	max := 20
	if termHeight > 0 {
		if h := termHeight - 6; h > 0 {
			max = h
		} else {
			max = 1
		}
	}
	if rowCount+1 < max {
		return rowCount + 1
	}
	return max
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.table.SetColumns(columnsForWidth(msg.Width))
		m.table.SetWidth(msg.Width)
		m.table.SetHeight(tableHeight(msg.Height, len(m.packages)))
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
	if m.quitting {
		return ""
	}
	var b strings.Builder
	ctx := ""
	if n := len(m.packages); n > 0 {
		ctx = fmt.Sprintf("%d package", n)
		if n != 1 {
			ctx += "s"
		}
	}
	b.WriteString(theme.Screen("Installed", ctx))
	b.WriteString("\n\n")
	if len(m.packages) == 0 {
		b.WriteString(theme.EmptyState("no packages installed — try `gale install <pkg>`"))
	} else {
		b.WriteString(m.table.View())
	}
	b.WriteString("\n\n")
	b.WriteString(theme.Help("↑↓", "navigate", "q", "quit"))
	return b.String()
}

func humanSize(n int64) string {
	if n <= 0 {
		return "—"
	}
	return theme.HumanBytes(n)
}

func Plain(packages []db.InstalledPackage) string {
	if len(packages) == 0 {
		return "no packages installed\n"
	}
	var out strings.Builder
	for _, p := range packages {
		fmt.Fprintf(&out, "%-20s %-12s %s\n", p.Name, p.Version, p.InstalledAt.Format("2006-01-02"))
	}
	return out.String()
}
