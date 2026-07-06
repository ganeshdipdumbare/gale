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
				tm := tree.NewModel(graph, args[0])
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

			events := make(chan download.Event, 128)
			a.Installer.Progress = events
			a.Installer.MaxJobs = a.Jobs

			names := make([]string, 0, len(graph.Order))
			for _, k := range graph.Order {
				names = append(names, graph.Nodes[k].Name)
			}

			ctx := context.Background()
			var installErr error
			if tui.IsInteractive(nt) {
				pm := progress.NewModel(names, events)
				prog := tea.NewProgram(pm, tea.WithAltScreen())
				go func() {
					coord := &resolver.InstallCoordinator{Installer: a.Installer}
					installErr = coord.Run(ctx, graph)
					close(events)
				}()
				final, err := prog.Run()
				if err != nil {
					return err
				}
				if m, ok := final.(progress.Model); ok && m.Err() != nil {
					return m.Err()
				}
				return installErr
			}

			a.Installer.OnState = func(pkg string, state download.State) {
				fmt.Fprintf(os.Stdout, "%s: %s\n", pkg, state)
			}
			coord := &resolver.InstallCoordinator{Installer: a.Installer}
			go func() {
				for ev := range events {
					if ev.State == download.StateDownloading && ev.TotalBytes > 0 {
						fmt.Fprintf(os.Stdout, "%s: %.0f%%\n", ev.ID, float64(ev.BytesDone)/float64(ev.TotalBytes)*100)
					}
				}
			}()
			return coord.Run(ctx, graph)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}
