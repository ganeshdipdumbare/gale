package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/tui"
	"github.com/ganeshdipdumbare/gale/internal/tui/search"
	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Fuzzy search packages",
		Args:  cobra.ExactArgs(1),
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

			if tui.IsInteractive(nt) {
				m := search.NewModel(a.Index, args[0])
				p := tea.NewProgram(m, tea.WithAltScreen())
				final, err := p.Run()
				if err != nil {
					return err
				}
				if fm, ok := final.(search.Model); ok {
					if sel := fm.Selected(); sel != nil {
						fmt.Printf("%s %s\n", sel.Name, sel.Version)
					}
				}
				return nil
			}

			for _, r := range a.Index.Search(args[0], 20) {
				fmt.Fprintf(os.Stdout, "%-20s %s\n", r.Package.Name, r.Package.Description)
			}
			return nil
		},
	}
}
