package progress

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

// TickMsg triggers UI refresh for speed/ETA smoothing.
type TickMsg time.Time

// EventMsg wraps a download event for Bubble Tea.
type EventMsg download.Event

// PkgProgress tracks one package row.
type PkgProgress struct {
	ID            string
	Name          string
	State         download.State
	BytesDone     int64
	TotalBytes    int64
	Speed         float64
	ETA           time.Duration
	ActiveWorkers int
	Err           error
}

// Model is the install progress TUI.
type Model struct {
	packages   map[string]*PkgProgress
	order      []string
	width      int
	height     int
	aggregate  progress.Model
	perPkg     map[string]progress.Model
	done       bool
	err        error
	quitting   bool
	useMock    bool
	events     <-chan download.Event
}

// NewModel creates a progress view for the given package names.
func NewModel(names []string, events <-chan download.Event) Model {
	m := Model{
		packages: make(map[string]*PkgProgress, len(names)),
		order:    append([]string(nil), names...),
		perPkg:   make(map[string]progress.Model),
		events:   events,
		aggregate: progress.New(
			progress.WithDefaultGradient(),
			progress.WithWidth(40),
		),
	}
	for _, n := range names {
		m.packages[n] = &PkgProgress{ID: n, Name: n, State: download.StateQueued}
		m.perPkg[n] = progress.New(progress.WithDefaultGradient(), progress.WithWidth(40))
	}
	return m
}

// NewMockModel creates a progress view driven by a fake event stream.
func NewMockModel(names []string) Model {
	ch := make(chan download.Event, 64)
	m := NewModel(names, ch)
	m.useMock = true
	go mockDownload(ch, names)
	return m
}

func mockDownload(ch chan<- download.Event, names []string) {
	defer close(ch)
	total := int64(50 * 1024 * 1024)
	for _, name := range names {
		var done int64
		workers := 2
		ch <- download.Event{ID: name, State: download.StateDownloading, TotalBytes: total, ActiveWorkers: workers}
		for done < total {
			step := int64(2 * 1024 * 1024)
			if done+step > total {
				step = total - done
			}
			done += step
			time.Sleep(80 * time.Millisecond)
			speed := float64(8 * 1024 * 1024)
			eta := time.Duration(float64(total-done)/speed) * time.Second
			ch <- download.Event{
				ID: name, State: download.StateDownloading,
				BytesDone: done, TotalBytes: total,
				Speed: speed, ETA: eta, ActiveWorkers: workers,
			}
		}
		ch <- download.Event{ID: name, State: download.StateVerifying, BytesDone: total, TotalBytes: total}
		time.Sleep(100 * time.Millisecond)
		ch <- download.Event{ID: name, State: download.StateInstalling, BytesDone: total, TotalBytes: total}
		time.Sleep(100 * time.Millisecond)
		ch <- download.Event{ID: name, State: download.StateDone, BytesDone: total, TotalBytes: total}
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		waitForEvent(m.events),
		tick(),
	)
}

func tick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg { return TickMsg(t) })
}

func waitForEvent(events <-chan download.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return tea.Quit()
		}
		return EventMsg(ev)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := msg.Width - 20
		if w < 20 {
			w = 20
		}
		m.aggregate = progress.New(progress.WithDefaultGradient(), progress.WithWidth(w))
		for k := range m.perPkg {
			m.perPkg[k] = progress.New(progress.WithDefaultGradient(), progress.WithWidth(w))
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	case TickMsg:
		return m, tick()
	case EventMsg:
		ev := download.Event(msg)
		p, ok := m.packages[ev.ID]
		if !ok {
			p = &PkgProgress{ID: ev.ID, Name: ev.ID}
			m.packages[ev.ID] = p
			m.order = append(m.order, ev.ID)
			m.perPkg[ev.ID] = progress.New(progress.WithDefaultGradient(), progress.WithWidth(40))
		}
		p.State = ev.State
		p.BytesDone = ev.BytesDone
		p.TotalBytes = ev.TotalBytes
		p.Speed = ev.Speed
		p.ETA = ev.ETA
		p.ActiveWorkers = ev.ActiveWorkers
		p.Err = ev.Error
		if ev.State == download.StateFailed {
			m.err = ev.Error
			m.done = true
			return m, tea.Quit
		}
		allDone := true
		for _, pkg := range m.packages {
			if pkg.State != download.StateDone {
				allDone = false
				break
			}
		}
		if allDone && len(m.packages) > 0 {
			m.done = true
			return m, tea.Quit
		}
		return m, waitForEvent(m.events)
	}
	return m, nil
}

func (m Model) View() string {
	if m.quitting {
		return theme.Muted.Render("cancelled\n")
	}
	var b strings.Builder
	b.WriteString(theme.Title.Render("gale install"))
	b.WriteString("\n\n")

	var aggDone, aggTotal int64
	for _, id := range m.order {
		p := m.packages[id]
		if p.TotalBytes > 0 {
			aggTotal += p.TotalBytes
		}
		aggDone += p.BytesDone
	}
	aggPct := 0.0
	if aggTotal > 0 {
		aggPct = float64(aggDone) / float64(aggTotal)
	}
	bar := m.aggregate.ViewAs(aggPct)
	b.WriteString(theme.Subtitle.Render("Overall"))
	b.WriteString("\n")
	b.WriteString(bar)
	fmt.Fprintf(&b, "  %s / %s\n\n", humanBytes(aggDone), humanBytes(aggTotal))

	for _, id := range m.order {
		p := m.packages[id]
		stateStr := string(p.State)
		b.WriteString(theme.Accent.Render(p.Name))
		b.WriteString(" ")
		b.WriteString(theme.StatusStyle(stateStr).Render(stateStr))
		b.WriteString("\n")
		pct := 0.0
		if p.TotalBytes > 0 {
			pct = float64(p.BytesDone) / float64(p.TotalBytes)
		}
		pb := m.perPkg[id]
		b.WriteString(pb.ViewAs(pct))
		fmt.Fprintf(&b, "  %5.1f%%  %s / %s  %s  ETA %s\n",
			pct*100,
			humanBytes(p.BytesDone), humanBytes(p.TotalBytes),
			humanSpeed(p.Speed), p.ETA.Round(time.Second))
		b.WriteString(workerDots(p.ActiveWorkers))
		b.WriteString("\n\n")
	}

	if m.done && m.err == nil {
		b.WriteString(theme.StatusDone.Render("✓ all packages installed\n"))
	} else if m.err != nil {
		b.WriteString(theme.StatusFailed.Render(fmt.Sprintf("✗ %v\n", m.err)))
	}
	b.WriteString(theme.Muted.Render("q quit"))
	return b.String()
}

func workerDots(n int) string {
	if n <= 0 {
		return theme.Muted.Render("workers: ·")
	}
	dots := strings.Repeat("●", n)
	if n > 8 {
		dots = strings.Repeat("●", 8) + fmt.Sprintf("+%d", n-8)
	}
	return theme.Muted.Render("workers: ") + theme.StatusDownloading.Render(dots)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func humanSpeed(bps float64) string {
	if bps <= 0 {
		return "— MB/s"
	}
	return fmt.Sprintf("%.1f MB/s", bps/(1024*1024))
}

// BridgeEvents returns a tea.Cmd that forwards download events to the program.
func BridgeEvents(p *tea.Program, events <-chan download.Event) {
	go func() {
		for ev := range events {
			p.Send(EventMsg(ev))
		}
	}()
}

// Err returns terminal error if any.
func (m Model) Err() error { return m.err }

// Done reports completion.
func (m Model) Done() bool { return m.done }
