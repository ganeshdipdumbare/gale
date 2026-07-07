// Package theme centralizes the visual language shared by every gale TUI
// screen: colors, text styles, and small rendering helpers (bars, truncation,
// panels, help hints) so screens stay visually consistent.
package theme

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	Fg     = lipgloss.AdaptiveColor{Light: "#1f2328", Dark: "#e6edf3"}
	Dim    = lipgloss.AdaptiveColor{Light: "#656d76", Dark: "#9198a1"}
	Faint  = lipgloss.AdaptiveColor{Light: "#afb8c1", Dark: "#6e7681"}
	Accent = lipgloss.AdaptiveColor{Light: "#0969da", Dark: "#4493f8"}
	OK     = lipgloss.AdaptiveColor{Light: "#1a7f37", Dark: "#3fb950"}
	Warn   = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#d29922"}
	Err    = lipgloss.AdaptiveColor{Light: "#cf222e", Dark: "#f85149"}
	Sel    = lipgloss.AdaptiveColor{Light: "#eaeef2", Dark: "#26314a"}
	Border = lipgloss.AdaptiveColor{Light: "#d0d7de", Dark: "#30363d"}
)

var (
	Bold     = lipgloss.NewStyle().Bold(true).Foreground(Fg)
	Text     = lipgloss.NewStyle().Foreground(Fg)
	Muted    = lipgloss.NewStyle().Foreground(Dim)
	FaintS   = lipgloss.NewStyle().Foreground(Faint)
	AccentS  = lipgloss.NewStyle().Foreground(Accent)
	AccentBS = lipgloss.NewStyle().Foreground(Accent).Bold(true)
	OKS      = lipgloss.NewStyle().Foreground(OK)
	OKBS     = lipgloss.NewStyle().Foreground(OK).Bold(true)
	WarnS    = lipgloss.NewStyle().Foreground(Warn)
	ErrS     = lipgloss.NewStyle().Foreground(Err)
	ErrBS    = lipgloss.NewStyle().Foreground(Err).Bold(true)
	SelS     = lipgloss.NewStyle().Foreground(Fg).Background(Sel).Bold(true)
)

// MinWidth/MinHeight are the smallest terminal dimensions we actively lay
// out for; below this we still render but stop trying to be clever about
// column/bar sizing.
const (
	MinWidth  = 20
	MinHeight = 6
)

// Spinner frames (braille, same family as hyperfine).
var Spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func SpinnerFrame(tick int) string {
	if len(Spinner) == 0 {
		return ""
	}
	return AccentS.Render(Spinner[tick%len(Spinner)])
}

// DotBar is a hyperfine-style █/░ progress bar.
func DotBar(ratio float64, width int) string {
	if width < 4 {
		width = 4
	}
	if ratio < 0 || ratio != ratio { // NaN guard
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	n := int(ratio*float64(width) + 0.5)
	if n > width {
		n = width
	}
	if n < 0 {
		n = 0
	}
	return AccentS.Render(strings.Repeat("█", n)) + FaintS.Render(strings.Repeat("░", width-n))
}

// ETA formats duration as HH:MM:SS like hyperfine.
func ETA(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// DoneLine formats a completed package row.
func DoneLine(name string, secs float64) string {
	return fmt.Sprintf("  %-20s %6.3f s", name, secs)
}

// Screen renders a consistent title: bold label, optional dim context.
func Screen(title, context string) string {
	s := Bold.Render(title)
	if context != "" {
		s += " " + Muted.Render(context)
	}
	return s
}

// Truncate shortens s to fit within max terminal columns (display width,
// not byte length), appending an ellipsis when it doesn't fit. Safe for
// wide (CJK) and multi-byte runes.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 1 {
		return runewidth.Truncate(s, max, "")
	}
	return runewidth.Truncate(s, max, "…")
}

// Pad truncates or right-pads a string to an exact display width.
func Pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = Truncate(s, w)
	gap := w - runewidth.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// PadLeft right-aligns a string within an exact display width.
func PadLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = Truncate(s, w)
	gap := w - runewidth.StringWidth(s)
	if gap <= 0 {
		return s
	}
	return strings.Repeat(" ", gap) + s
}

// Help is a single-line key hint bar.
func Help(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	var out []string
	for i := 0; i < len(parts); i += 2 {
		if i+1 >= len(parts) {
			break
		}
		out = append(out, Bold.Render(parts[i])+" "+Muted.Render(parts[i+1]))
	}
	return FaintS.Render(strings.Join(out, "   "))
}

// Divider draws a full-width horizontal rule, useful to separate a header
// from scrollable content.
func Divider(width int) string {
	if width <= 0 {
		width = 1
	}
	return FaintS.Render(strings.Repeat("─", width))
}

// ScrollHint renders a small "more above/below" indicator for scrollable
// regions so users know there's hidden content.
func ScrollHint(above, below bool, width int) string {
	if !above && !below {
		return ""
	}
	label := ""
	switch {
	case above && below:
		label = "↑↓ more"
	case above:
		label = "↑ more above"
	case below:
		label = "↓ more below"
	}
	return FaintS.Render(Pad(label, width))
}

// EmptyState renders a centered-ish muted placeholder message for screens
// with no content (no results, nothing installed, empty tree, ...).
func EmptyState(msg string) string {
	return "\n" + Muted.Render("  "+msg) + "\n"
}

// HumanBytes formats a byte count as a short human-readable size.
func HumanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	units := "KMGTPE"
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), units[exp])
}

// HumanSpeed formats bytes/sec as a short human-readable rate.
func HumanSpeed(bytesPerSec float64) string {
	if bytesPerSec < 0 || bytesPerSec != bytesPerSec {
		bytesPerSec = 0
	}
	return HumanBytes(int64(bytesPerSec)) + "/s"
}
