package cmd

import (
	"fmt"

	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/spf13/cobra"
)

func newRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <pkg>...",
		Short: "Remove installed packages",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nt, j, v := flags()
			a, err := app.Open(nt, j, v)
			if err != nil {
				return err
			}
			defer a.Close()
			for _, name := range args {
				if err := a.Database.RemoveInstall(name); err != nil {
					return err
				}
				fmt.Printf("removed %s\n", name)
			}
			return nil
		},
	}
}
