// Package progress renders the live install/download progress screen shown
// while `gale install` fetches and links packages.
package progress

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/tui/theme"
)

type TickMsg time.Time

type EventMsg download.Event

type FinishedMsg struct {
	Err error
}

type PkgProgress struct {
	ID         string
	Name       string
	State      download.State
	BytesDone  int64
	TotalBytes int64
	Speed      float64
	ETA        time.Duration
	StartedAt  time.Time
	FinishedAt time.Time
	Duration   time.Duration
	Err        error
}

// maxActiveRows caps how many concurrently-active packages get their own
// progress line. In practice this is bounded by Installer.MaxJobs, but the
// cap keeps the layout sane even if callers raise concurrency very high.
const maxActiveRows = 8

type Model struct {
	packages map[string]*PkgProgress
	order    []string
	width    int
	height   int
	tick     int
	started  time.Time
	finished bool
	err      error
	quitting bool
}

func NewModel(names []string) Model {
	m := Model{
		packages: make(map[string]*PkgProgress, len(names)),
		order:    append([]string(nil), names...),
		started:  time.Now(),
	}
	for _, n := range names {
		m.packages[n] = &PkgProgress{ID: n, Name: n, State: download.StateQueued}
	}
	if len(names) == 0 {
		m.finished = true
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.finished {
		return nil
	}
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return TickMsg(t) })
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	case TickMsg:
		if m.finished {
			return m, nil
		}
		m.tick++
		return m, tick()
	case EventMsg:
		ev := download.Event(msg)
		m.applyEvent(ev)
		if ev.State == download.StateFailed {
			m.err = ev.Error
			m.finished = true
			return m, tea.Quit
		}
	case FinishedMsg:
		m.finished = true
		m.err = msg.Err
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) applyEvent(ev download.Event) {
	p, ok := m.packages[ev.ID]
	if !ok && ev.ID != "" {
		p = &PkgProgress{ID: ev.ID, Name: ev.ID}
		m.packages[ev.ID] = p
		m.order = append(m.order, ev.ID)
	} else if !ok {
		return
	}

	prev := p.State
	if ev.State != "" {
		p.State = ev.State
	}
	if ev.BytesDone > 0 || ev.TotalBytes > 0 {
		p.BytesDone = ev.BytesDone
		p.TotalBytes = ev.TotalBytes
	}
	if ev.Speed > 0 {
		p.Speed = ev.Speed
	}
	if ev.ETA > 0 {
		p.ETA = ev.ETA
	}
	if ev.Error != nil {
		p.Err = ev.Error
	}

	// Track timing for done lines.
	if prev != download.StateDownloading && ev.State == download.StateDownloading {
		p.StartedAt = time.Now()
	}
	if prev != download.StateDone && ev.State == download.StateDone {
		p.FinishedAt = time.Now()
		if !p.StartedAt.IsZero() {
			p.Duration = p.FinishedAt.Sub(p.StartedAt)
		}
		if p.TotalBytes > 0 {
			p.BytesDone = p.TotalBytes
		}
	}
	if prev != download.StateDone && (ev.State == download.StateInstalling || ev.State == download.StateVerifying) {
		if p.StartedAt.IsZero() {
			p.StartedAt = time.Now()
		}
	}
}

// counts returns the number of packages in each terminal/transient bucket.
func (m Model) counts() (done, active, queued, failed int) {
	for _, id := range m.order {
		switch m.packages[id].State {
		case download.StateDone:
			done++
		case download.StateQueued, "":
			queued++
		case download.StateFailed:
			failed++
		default:
			active++
		}
	}
	return
}

func (m Model) totalBytesProgress() (done, total int64) {
	for _, id := range m.order {
		p := m.packages[id]
		if p.TotalBytes > 0 {
			done += p.BytesDone
			total += p.TotalBytes
		}
	}
	return
}

func (m Model) View() string {
	if m.quitting {
		return theme.Muted.Render("Cancelled.") + "\n"
	}
	if len(m.order) == 0 {
		return theme.Screen("Install", "") + "\n" + theme.EmptyState("nothing to install — already up to date") + "\n"
	}

	width := m.width
	if width <= 0 {
		width = 80
	}

	doneCount, activeCount, queuedCount, failedCount := m.counts()
	total := len(m.order)

	var b strings.Builder
	b.WriteString(theme.Screen("Install", fmt.Sprintf("%d/%d packages", doneCount, total)))
	b.WriteString("\n")

	overallRatio := float64(doneCount) / float64(total)
	if bd, bt := m.totalBytesProgress(); bt > 0 {
		// Blend byte-level progress into the overall ratio so a single large
		// package doesn't look "stuck" at the same percentage as a fully
		// finished small one.
		byteRatio := float64(bd) / float64(bt)
		overallRatio = (overallRatio*float64(total) + byteRatio) / float64(total+1)
	}
	barW := width - len(" 100%") - 1
	if barW < 8 {
		barW = 8
	}
	b.WriteString(theme.DotBar(overallRatio, barW))
	b.WriteString(theme.Muted.Render(fmt.Sprintf(" %3.0f%%", overallRatio*100)))
	b.WriteString("\n\n")

	// Completed rows — hyperfine result style. Cap how many we print so a
	// long install (dozens of packages) doesn't scroll the active section
	// off-screen; show the most recent ones and summarize the rest.
	completed := m.completedRows()
	maxDoneRows := m.availableDoneRows(activeCount, queuedCount)
	shown := completed
	hiddenDone := 0
	if maxDoneRows >= 0 && len(shown) > maxDoneRows {
		hiddenDone = len(shown) - maxDoneRows
		shown = shown[hiddenDone:]
	}
	if hiddenDone > 0 {
		b.WriteString(theme.FaintS.Render(fmt.Sprintf("  … %d more completed above", hiddenDone)))
		b.WriteString("\n")
	}
	for _, p := range shown {
		b.WriteString(m.doneLine(p, width))
		b.WriteString("\n")
	}

	// Active rows — every package currently downloading/verifying/installing,
	// since installs run several packages in parallel per dependency level.
	if !m.finished {
		activeRows := m.activePackages()
		if len(activeRows) > 0 {
			b.WriteString("\n")
			activeCap := maxActiveRows
			if activeCap > len(activeRows) {
				activeCap = len(activeRows)
			}
			for _, p := range activeRows[:activeCap] {
				b.WriteString(m.activeLine(p, width))
				b.WriteString("\n")
			}
			if extra := len(activeRows) - activeCap; extra > 0 {
				b.WriteString(theme.FaintS.Render(fmt.Sprintf("  … +%d more in progress", extra)))
				b.WriteString("\n")
			}
		}
		if queuedCount > 0 {
			b.WriteString("\n")
			b.WriteString(theme.Muted.Render(fmt.Sprintf("  %d queued", queuedCount)))
			b.WriteString("\n")
		}
	}

	if m.finished {
		b.WriteString("\n")
		switch {
		case m.err != nil:
			b.WriteString(theme.ErrBS.Render("Failed  ") + theme.ErrS.Render(m.err.Error()))
		case failedCount > 0:
			b.WriteString(theme.ErrBS.Render(fmt.Sprintf("%d package(s) failed", failedCount)))
		default:
			elapsed := time.Since(m.started).Seconds()
			b.WriteString(theme.OKBS.Render("Done  ") + theme.OKS.Render(fmt.Sprintf("%d packages in %.3f s", doneCount, elapsed)))
		}
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(theme.Help("q", "cancel"))
	}

	return b.String()
}

// availableDoneRows estimates how many completed rows fit given the known
// terminal height, reserving space for header, active rows, and footer.
// Returns -1 (unbounded) when height is unknown.
func (m Model) availableDoneRows(activeCount, queuedCount int) int {
	if m.height <= 0 {
		return -1
	}
	reserved := 4 // title + bar + blank + footer
	if activeCount > 0 {
		reserved += min(activeCount, maxActiveRows) + 1
		if activeCount > maxActiveRows {
			reserved++
		}
	}
	if queuedCount > 0 {
		reserved += 2
	}
	avail := m.height - reserved
	if avail < 1 {
		avail = 1
	}
	return avail
}

func (m Model) completedRows() []*PkgProgress {
	var out []*PkgProgress
	for _, id := range m.order {
		p := m.packages[id]
		if p.State == download.StateDone {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) activePackages() []*PkgProgress {
	var out []*PkgProgress
	for _, id := range m.order {
		p := m.packages[id]
		switch p.State {
		case download.StateDownloading, download.StateVerifying, download.StateInstalling:
			out = append(out, p)
		}
	}
	return out
}

func (m Model) doneLine(p *PkgProgress, width int) string {
	secs := p.Duration.Seconds()
	if secs <= 0 && !p.FinishedAt.IsZero() && !p.StartedAt.IsZero() {
		secs = p.FinishedAt.Sub(p.StartedAt).Seconds()
	}
	if secs <= 0 {
		secs = 0.001
	}
	nameW := width - len("   999.999 s") - 3
	if nameW < 8 {
		nameW = 8
	}
	return theme.Muted.Render("  ✓ ") + theme.Text.Render(theme.Pad(p.Name, nameW)) +
		theme.Muted.Render(fmt.Sprintf(" %6.3f s", secs))
}

func (m Model) activeLine(p *PkgProgress, width int) string {
	label := activityLabel(p)
	pct := 0.0
	if p.TotalBytes > 0 {
		pct = float64(p.BytesDone) / float64(p.TotalBytes)
	}

	nameW := 20
	statusW := 12
	etaW := len(" ETA 00:00:00")
	spinW := 2
	barW := width - spinW - nameW - statusW - etaW - 4
	if barW < 6 {
		barW = 6
	}

	var b strings.Builder
	b.WriteString(theme.SpinnerFrame(m.tick))
	b.WriteString(" ")
	b.WriteString(theme.Text.Render(theme.Pad(p.Name, nameW)))
	b.WriteString(" ")
	b.WriteString(theme.AccentS.Render(theme.Pad(label, statusW)))
	b.WriteString(theme.DotBar(pct, barW))
	if p.TotalBytes > 0 {
		b.WriteString(theme.Muted.Render(" ETA "))
		b.WriteString(theme.Text.Render(theme.ETA(p.ETA)))
	} else {
		b.WriteString(theme.Muted.Render(strings.Repeat(" ", etaW)))
	}
	return b.String()
}

func activityLabel(p *PkgProgress) string {
	switch p.State {
	case download.StateQueued:
		return "waiting"
	case download.StateDownloading:
		return "downloading"
	case download.StateVerifying:
		return "verifying"
	case download.StateInstalling:
		return "installing"
	default:
		return string(p.State)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Bridge forwards download events from the installer into the Bubble Tea
// program. It returns a channel that is closed once every buffered event
// has been forwarded and processed and the events channel has been closed.
//
// Callers that need to send a terminal message afterwards (e.g.
// FinishedMsg) must wait on this channel first: p.Send blocks until the
// program's event loop has received the message, so by the time the
// returned channel closes every prior EventMsg is guaranteed to have been
// applied. Sending FinishedMsg without this synchronization races with the
// drain and can let FinishedMsg (which quits the program) be processed
// before the final StateDone events, showing a stale/short completed count.
func Bridge(p *tea.Program, events <-chan download.Event) <-chan struct{} {
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		for ev := range events {
			p.Send(EventMsg(ev))
		}
	}()
	return drained
}

func (m Model) Err() error { return m.err }
