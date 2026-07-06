package search

import (
	"fmt"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

type item struct {
	pkg index.Package
}

func (i item) Title() string       { return i.pkg.Name }
func (i item) Description() string { return fmt.Sprintf("%s — %s", i.pkg.Version, i.pkg.Description) }
func (i item) FilterValue() string { return i.pkg.Name + " " + i.pkg.Description }

// Model is the fuzzy search browse view.
type Model struct {
	list     list.Model
	selected *index.Package
	quitting bool
	width    int
	height   int
}

func NewModel(idx *index.Index, query string) Model {
	var items []list.Item
	if idx != nil {
		for _, r := range idx.Search(query, 50) {
			items = append(items, item{pkg: r.Package})
		}
	}
	delegate := list.NewDefaultDelegate()
	l := list.New(items, delegate, 0, 0)
	l.Title = theme.Title.Render("gale search")
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	if query != "" {
		l.SetFilterText(query)
	}
	return Model{list: l}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 2)
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if msg.String() == "enter" {
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
	return lipgloss.NewStyle().Render(m.list.View())
}

func (m Model) Selected() *index.Package { return m.selected }
