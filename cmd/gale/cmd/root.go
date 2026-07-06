package cmd

import (
	"github.com/spf13/cobra"
)

var (
	noTUI   bool
	jobs    int
	verbose bool
)

// NewRoot builds the gale CLI.
func NewRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "gale",
		Short: "Fast, resumable macOS package manager",
		Long:  "gale is a native Go package manager with resumable parallel downloads and a Bubble Tea UI.",
	}
	root.PersistentFlags().BoolVar(&noTUI, "no-tui", false, "disable terminal UI")
	root.PersistentFlags().IntVar(&jobs, "jobs", 0, "max parallel download workers (default: NumCPU)")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "verbose logging")

	root.AddCommand(
		newInstallCmd(),
		newRemoveCmd(),
		newUpdateCmd(),
		newUpgradeCmd(),
		newSearchCmd(),
		newListCmd(),
		newInfoCmd(),
		newDoctorCmd(),
		newVersionCmd(version),
	)
	return root
}

func flags() (bool, int, bool) {
	return noTUI, jobs, verbose
}
