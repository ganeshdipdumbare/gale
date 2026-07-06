package cmd

import (
	"fmt"

	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Refresh the package index",
		RunE: func(cmd *cobra.Command, args []string) error {
			nt, j, v := flags()
			a, err := app.Open(nt, j, v)
			if err != nil {
				return err
			}
			defer a.Close()
			idx, changed, err := a.IndexClient.Update(force)
			if err != nil {
				return err
			}
			a.Index = idx
			fmt.Printf("index updated: %d packages changed\n", changed)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "force full re-download")
	return cmd
}
