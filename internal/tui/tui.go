package tui

import (
	"os"

	"golang.org/x/term"
)

// IsInteractive returns true when stdout is a TTY and TUI is allowed.
func IsInteractive(noTUI bool) bool {
	if noTUI {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}
