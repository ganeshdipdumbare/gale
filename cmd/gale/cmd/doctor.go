package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/paths"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Verify store integrity and symlinks",
		RunE: func(cmd *cobra.Command, args []string) error {
			nt, j, v := flags()
			a, err := app.Open(nt, j, v)
			if err != nil {
				return err
			}
			defer a.Close()

			records, err := a.Database.AllFileRecords()
			if err != nil {
				return err
			}
			issues := a.Store.Doctor(records)
			for _, issue := range issues {
				fmt.Fprintf(os.Stdout, "[%s] %s: %s\n", issue.Kind, issue.Path, issue.Message)
			}

			binDir, _ := paths.Bin()
			entries, _ := os.ReadDir(binDir)
			for _, e := range entries {
				link := filepath.Join(binDir, e.Name())
				target, err := os.Readlink(link)
				if err != nil {
					fmt.Fprintf(os.Stdout, "[symlink] %s: broken (%v)\n", link, err)
					continue
				}
				if _, err := os.Stat(target); os.IsNotExist(err) {
					fmt.Fprintf(os.Stdout, "[symlink] %s -> %s: target missing\n", link, target)
				}
			}

			if len(issues) == 0 {
				fmt.Fprintln(os.Stdout, "gale doctor: no issues found")
			}
			return nil
		},
	}
}
