package tui

import (
	"os"

	"golang.org/x/term"
)

// IsInteractive returns true when stdin/stdout are TTYs and TUI is allowed.
func IsInteractive(noTUI bool) bool {
	if noTUI {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}
