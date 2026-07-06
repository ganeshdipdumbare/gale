package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/tui"
	"github.com/ganeshdipdumbare/gale/internal/tui/detail"
	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <pkg>",
		Short: "Show package details",
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
			pkg, ok := a.Index.Get(args[0])
			if !ok {
				return fmt.Errorf("package %q not found", args[0])
			}
			if tui.IsInteractive(nt) {
				m := detail.NewModel(pkg)
				p := tea.NewProgram(m, tea.WithAltScreen())
				_, err := p.Run()
				return err
			}
			fmt.Fprint(os.Stdout, detail.Plain(pkg))
			return nil
		},
	}
}
