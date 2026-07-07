package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
	"github.com/ganeshdipdumbare/gale/internal/tui"
	"github.com/ganeshdipdumbare/gale/internal/tui/progress"
	"github.com/ganeshdipdumbare/gale/internal/tui/tree"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "install <pkg>...",
		Short: "Install packages and their dependencies",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nt, j, v := flags()
			a, err := app.Open(nt, j, v)
			if err != nil {
				return err
			}
			defer a.Close()
			if err := a.EnsureIndex(false); err != nil {
				return err
			}
			graph, err := resolver.BuildGraph(a.Index, args...)
			if err != nil {
				return err
			}

			if !yes && tui.IsInteractive(nt) {
				tm := tree.NewModel(graph, args...)
				p := tea.NewProgram(tm, tea.WithAltScreen())
				final, err := p.Run()
				if err != nil {
					return err
				}
				if m, ok := final.(tree.Model); !ok || !m.Confirmed() {
					return fmt.Errorf("install cancelled")
				}
			} else if !yes {
				fmt.Fprintf(os.Stdout, "will install: %s\n", strings.Join(graph.Order, ", "))
			}

			names := make([]string, 0, len(graph.Order))
			for _, k := range graph.Order {
				names = append(names, graph.Nodes[k].Name)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if tui.IsInteractive(nt) {
				events := make(chan download.Event, 256)
				a.Installer.Progress = events
				a.Installer.MaxJobs = a.Jobs
				// OnState is used by plain mode; emit() also writes to Progress for TUI.
				a.Installer.OnState = nil

				pm := progress.NewModel(names)
				prog := tea.NewProgram(pm, tea.WithAltScreen())
				drained := progress.Bridge(prog, events)

				// installResult carries the install outcome from the
				// background goroutine. Using a channel (instead of a plain
				// shared variable) avoids a data race with the read below:
				// if the user quits the TUI before the install finishes,
				// prog.Run() returns while this goroutine is still running,
				// so a bare `installErr` read/write pair would race.
				installResult := make(chan error, 1)
				go func() {
					coord := &resolver.InstallCoordinator{Installer: a.Installer}
					err := coord.Run(ctx, graph)
					close(events)
					// Wait for every buffered progress event to be forwarded
					// and applied before declaring the install finished, so
					// the final view can't show a stale/short done count.
					<-drained
					installResult <- err
					prog.Send(progress.FinishedMsg{Err: err})
				}()

				final, runErr := prog.Run()
				installErr := resolveInstallResult(cancel, installResult)

				if runErr != nil {
					return runErr
				}
				if m, ok := final.(progress.Model); ok && m.Err() != nil {
					return m.Err()
				}
				return installErr
			}

			events := make(chan download.Event, 64)
			a.Installer.Progress = events
			a.Installer.MaxJobs = a.Jobs
			a.Installer.OnState = func(pkg string, state download.State) {
				fmt.Fprintf(os.Stdout, "%s: %s\n", pkg, state)
			}
			go func() {
				for ev := range events {
					if ev.State == download.StateDownloading && ev.TotalBytes > 0 {
						fmt.Fprintf(os.Stdout, "%s: %.0f%%\n", ev.ID, float64(ev.BytesDone)/float64(ev.TotalBytes)*100)
					}
				}
			}()
			coord := &resolver.InstallCoordinator{Installer: a.Installer}
			return coord.Run(ctx, graph)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

// resolveInstallResult determines the final install error once the progress
// TUI has exited. installResult must be a channel the install goroutine
// sends its single result to after it fully finishes (see the goroutine
// above); reading it (rather than a plain shared variable) is what makes
// this race-free regardless of why/when the TUI exited.
//
// If the install already finished (or finishes concurrently), its result is
// returned immediately. If the TUI exited first — e.g. the user pressed
// q/ctrl+c while the install was still running — cancel is invoked and this
// blocks until the install goroutine actually stops, so the caller never
// returns (and runs `defer a.Close()`) while that goroutine might still be
// touching the DB/store.
func resolveInstallResult(cancel context.CancelFunc, installResult <-chan error) error {
	select {
	case err := <-installResult:
		return err
	default:
		cancel()
		<-installResult
		return fmt.Errorf("install cancelled")
	}
}
