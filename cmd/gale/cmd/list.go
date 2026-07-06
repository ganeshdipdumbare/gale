package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/tui"
	"github.com/ganeshdipdumbare/gale/internal/tui/tableview"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			nt, j, v := flags()
			a, err := app.Open(nt, j, v)
			if err != nil {
				return err
			}
			defer a.Close()
			pkgs, err := a.Database.ListInstalled()
			if err != nil {
				return err
			}
			if tui.IsInteractive(nt) {
				m := tableview.NewModel(pkgs)
				p := tea.NewProgram(m, tea.WithAltScreen())
				_, err := p.Run()
				return err
			}
			fmt.Fprint(os.Stdout, tableview.Plain(pkgs))
			return nil
		},
	}
}
