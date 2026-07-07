// Package search renders the `gale search` screen: a filterable list of
// packages matching a query.
package search

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// maxResults caps how many matches we load into the list; searches rarely
// need more and it keeps filtering snappy on large indexes.
const maxResults = 50

type item struct {
	pkg index.Package
}

func (i item) Title() string       { return i.pkg.Name }
func (i item) Description() string { return i.pkg.Version }
func (i item) FilterValue() string { return i.pkg.Name + " " + i.pkg.Description + " " + i.pkg.Version }

type Model struct {
	list     list.Model
	selected *index.Package
	quitting bool
	width    int
	height   int
	empty    bool
	query    string
}

func NewModel(idx *index.Index, query string) Model {
	var items []list.Item
	if idx != nil {
		for _, r := range idx.Search(query, maxResults) {
			items = append(items, item{pkg: r.Package})
		}
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = true
	d.SetSpacing(1)
	d.Styles.NormalTitle = theme.Text
	d.Styles.NormalDesc = theme.Muted
	d.Styles.SelectedTitle = theme.SelS.PaddingLeft(1)
	d.Styles.SelectedDesc = lipgloss.NewStyle().Foreground(theme.Dim).Background(theme.Sel).PaddingLeft(1)
	d.Styles.FilterMatch = theme.AccentBS.Underline(true)

	l := list.New(items, d, 0, 0)
	l.Title = theme.Screen("Search", queryContext(query, len(items)))
	l.Styles.Title = lipgloss.NewStyle().MarginBottom(1)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	if query != "" {
		l.SetFilterText(query)
	}
	return Model{list: l, empty: len(items) == 0, query: query}
}

func queryContext(query string, n int) string {
	if query == "" {
		return ""
	}
	return fmt.Sprintf("%d result(s) for %q", n, query)
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		listHeight := msg.Height - 3
		if listHeight < 1 {
			listHeight = 1
		}
		m.list.SetHeight(listHeight)
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "enter" && !m.empty {
			if it, ok := m.list.SelectedItem().(item); ok {
				pkg := it.pkg
				m.selected = &pkg
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.empty {
		msg := "no packages found"
		if m.query != "" {
			msg = fmt.Sprintf("no packages found for %q", m.query)
		}
		return theme.Screen("Search", "") + "\n" + theme.EmptyState(msg) + "\n" + theme.Help("q", "quit")
	}

	// Show description of selected item below the list when space allows.
	view := m.list.View()
	if it, ok := m.list.SelectedItem().(item); ok && it.pkg.Description != "" {
		desc := it.pkg.Description
		if m.width > 0 {
			desc = theme.Truncate(desc, m.width)
		}
		view += "\n" + theme.Muted.Render(desc)
	}
	view += "\n\n" + theme.Help("enter", "select", "/", "filter", "q", "quit")
	return view
}

func (m Model) Selected() *index.Package { return m.selected }
