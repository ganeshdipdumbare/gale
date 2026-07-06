package theme

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "#1a1a2e", Dark: "#e0e0ff"})

	Subtitle = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#444", Dark: "#aaa"})

	Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.AdaptiveColor{Light: "#888", Dark: "#555"}).
		Padding(0, 1)

	StatusQueued = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#666", Dark: "#888"})

	StatusDownloading = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#0066cc", Dark: "#5dade2"})

	StatusVerifying = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#cc6600", Dark: "#f39c12"})

	StatusInstalling = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#6600cc", Dark: "#bb8fce"})

	StatusDone = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1e8449", Dark: "#58d68d"})

	StatusFailed = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#c0392b", Dark: "#e74c3c"})

	Accent = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#1a5276", Dark: "#85c1e9"})

	Muted = lipgloss.NewStyle().
		Foreground(lipgloss.AdaptiveColor{Light: "#999", Dark: "#666"})
)

func StatusStyle(state string) lipgloss.Style {
	switch state {
	case "queued":
		return StatusQueued
	case "downloading":
		return StatusDownloading
	case "verifying":
		return StatusVerifying
	case "installing":
		return StatusInstalling
	case "done":
		return StatusDone
	case "failed":
		return StatusFailed
	default:
		return Muted
	}
}
