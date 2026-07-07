package tableview

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/db"
)

func TestColumnsForWidthAdaptsNameColumn(t *testing.T) {
	narrow := columnsForWidth(50)
	wide := columnsForWidth(200)
	if narrow[0].Width >= wide[0].Width {
		t.Errorf("wider terminal should get a wider name column: narrow=%d wide=%d", narrow[0].Width, wide[0].Width)
	}
	if narrow[0].Width < minNameCol {
		t.Errorf("name column should never shrink below the minimum, got %d", narrow[0].Width)
	}
}

func TestColumnsForWidthZeroUsesDefault(t *testing.T) {
	cols := columnsForWidth(0)
	if cols[0].Width <= 0 {
		t.Errorf("expected a sane default name width when terminal size is unknown, got %d", cols[0].Width)
	}
}

func TestTableHeightRespectsTerminal(t *testing.T) {
	if h := tableHeight(10, 100); h > 10 {
		t.Errorf("table height %d should not exceed reserved space for a 10-row terminal", h)
	}
	if h := tableHeight(0, 3); h != 4 {
		t.Errorf("with unknown terminal height and few rows, expect rows+1=4, got %d", h)
	}
	if h := tableHeight(2, 100); h < 1 {
		t.Errorf("table height should never be less than 1, got %d", h)
	}
}

func TestViewEmptyState(t *testing.T) {
	m := NewModel(nil)
	view := m.View()
	if !strings.Contains(view, "no packages installed") {
		t.Errorf("expected empty-state message for zero packages, got:\n%s", view)
	}
}

func TestViewNonEmptyShowsCount(t *testing.T) {
	pkgs := []db.InstalledPackage{
		{Name: "foo", Version: "1.0", InstalledAt: time.Now(), Size: 2048},
	}
	m := NewModel(pkgs)
	res, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := res.(Model)
	view := mm.View()
	if !strings.Contains(view, "1 package") {
		t.Errorf("expected package count in title, got:\n%s", view)
	}
	if !strings.Contains(view, "foo") {
		t.Errorf("expected package name in table, got:\n%s", view)
	}
}

func TestHumanSizeZeroIsDash(t *testing.T) {
	if got := humanSize(0); got != "—" {
		t.Errorf("humanSize(0) = %q, want em dash placeholder", got)
	}
}
