package progress

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/download"
)

func TestNewModelNoPackagesFinishesImmediately(t *testing.T) {
	m := NewModel(nil)
	if !m.finished {
		t.Fatal("a zero-package install should be marked finished immediately")
	}
	if cmd := m.Init(); cmd != nil {
		t.Error("Init should not schedule a tick when there is nothing to install")
	}
	view := m.View()
	if !strings.Contains(view, "nothing to install") {
		t.Errorf("expected an explicit empty-state message, got:\n%s", view)
	}
}

func TestApplyEventTracksMultipleActivePackages(t *testing.T) {
	m := NewModel([]string{"a", "b", "c"})
	m.applyEvent(download.Event{ID: "a", State: download.StateDownloading, BytesDone: 10, TotalBytes: 100})
	m.applyEvent(download.Event{ID: "b", State: download.StateInstalling})
	// c stays queued.

	active := m.activePackages()
	if len(active) != 2 {
		t.Fatalf("expected 2 active packages (parallel installs), got %d", len(active))
	}

	done, activeCount, queued, failed := m.counts()
	if done != 0 || activeCount != 2 || queued != 1 || failed != 0 {
		t.Errorf("counts() = done=%d active=%d queued=%d failed=%d, want 0,2,1,0", done, activeCount, queued, failed)
	}

	view := m.View()
	if !strings.Contains(view, "a") || !strings.Contains(view, "b") {
		t.Errorf("expected both active packages rendered simultaneously, got:\n%s", view)
	}
	if !strings.Contains(view, "1 queued") {
		t.Errorf("expected queued summary line, got:\n%s", view)
	}
}

func TestApplyEventUnknownIDIsTrackedDynamically(t *testing.T) {
	m := NewModel([]string{"a"})
	m.applyEvent(download.Event{ID: "unexpected-dep", State: download.StateDownloading})
	if _, ok := m.packages["unexpected-dep"]; !ok {
		t.Error("events for packages not in the initial name list should still be tracked")
	}
}

func TestApplyEventEmptyIDIgnored(t *testing.T) {
	m := NewModel([]string{"a"})
	before := len(m.order)
	m.applyEvent(download.Event{ID: "", State: download.StateDownloading})
	if len(m.order) != before {
		t.Error("an event with an empty ID should not create a phantom package row")
	}
}

func TestUpdateFailedEventFinishesAndQuits(t *testing.T) {
	m := NewModel([]string{"a"})
	failErr := errors.New("boom")
	res, cmd := m.Update(EventMsg(download.Event{ID: "a", State: download.StateFailed, Error: failErr}))
	mm := res.(Model)
	if !mm.finished {
		t.Error("a failed download should mark the model finished")
	}
	if mm.err == nil || mm.err.Error() != "boom" {
		t.Errorf("expected the failure error to be surfaced, got %v", mm.err)
	}
	if cmd == nil {
		t.Error("expected a quit command on failure")
	}
	view := mm.View()
	if !strings.Contains(view, "boom") {
		t.Errorf("expected error text in the finished view, got:\n%s", view)
	}
}

func TestFinishedTickStopsRescheduling(t *testing.T) {
	m := NewModel([]string{"a"})
	m.finished = true
	_, cmd := m.Update(TickMsg{})
	if cmd != nil {
		t.Error("no further ticks should be scheduled once the install is finished")
	}
}

func TestDoneRowsCappedOnSmallTerminal(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = "pkg" + string(rune('a'+i))
	}
	m := NewModel(names)
	for _, n := range names {
		m.applyEvent(download.Event{ID: n, State: download.StateDownloading})
		m.applyEvent(download.Event{ID: n, State: download.StateDone})
	}
	res, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	mm := res.(Model)

	view := mm.View()
	lineCount := strings.Count(view, "\n")
	if lineCount > 12+4 { // small slack for header/footer rounding
		t.Errorf("view has %d lines, expected it to respect the small terminal height (12):\n%s", lineCount, view)
	}
	if !strings.Contains(view, "more completed above") {
		t.Errorf("expected a truncation summary for the hidden completed rows, got:\n%s", view)
	}
}

func TestQuittingShowsCancelled(t *testing.T) {
	m := NewModel([]string{"a"})
	res, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := res.(Model)
	if !mm.quitting {
		t.Error("ctrl+c should set quitting")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
	if !strings.Contains(mm.View(), "Cancelled") {
		t.Errorf("expected cancelled message, got:\n%s", mm.View())
	}
}

// TestBridgeDrainsAllEventsBeforeSignalingDone is a regression test for a
// race where FinishedMsg could be processed before the last buffered
// EventMsgs, showing a stale/short completed count on a successful install.
// It runs a real tea.Program (headless: no input/renderer) and verifies
// that by the time Bridge's returned channel closes, every event sent
// through it has already been applied to the model — so callers who wait on
// that channel before sending FinishedMsg can't observe a partial state.
func TestBridgeDrainsAllEventsBeforeSignalingDone(t *testing.T) {
	names := []string{"a", "b", "c"}
	p := tea.NewProgram(NewModel(names),
		tea.WithInput(nil),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
		tea.WithoutSignals(),
	)

	events := make(chan download.Event, len(names)*2)
	for _, n := range names {
		events <- download.Event{ID: n, State: download.StateDownloading}
		events <- download.Event{ID: n, State: download.StateDone}
	}
	close(events)

	drained := Bridge(p, events)

	runDone := make(chan tea.Model, 1)
	go func() {
		final, _ := p.Run()
		runDone <- final
	}()

	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Bridge to drain all events")
	}

	// Only now is it safe to send a terminal message: every event above is
	// guaranteed to have already been applied to the running model.
	p.Send(FinishedMsg{})

	var final tea.Model
	select {
	case final = <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the program to quit")
	}

	fm, ok := final.(Model)
	if !ok {
		t.Fatalf("unexpected final model type %T", final)
	}
	done, active, queued, failed := fm.counts()
	if done != len(names) || active != 0 || queued != 0 || failed != 0 {
		t.Errorf("expected all %d packages done with none active/queued when FinishedMsg follows drain, got done=%d active=%d queued=%d failed=%d",
			len(names), done, active, queued, failed)
	}
}
