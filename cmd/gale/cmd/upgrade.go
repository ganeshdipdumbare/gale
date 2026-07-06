package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ganeshdipdumbare/gale/internal/app"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
	"github.com/spf13/cobra"
)

func newUpgradeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upgrade [pkg]",
		Short: "Upgrade installed packages",
		Args:  cobra.MaximumNArgs(1),
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

			var targets []string
			if len(args) == 1 {
				targets = []string{args[0]}
			} else {
				installed, err := a.Database.ListInstalled()
				if err != nil {
					return err
				}
				for _, p := range installed {
					targets = append(targets, p.Name)
				}
			}

			ctx := context.Background()
			for _, name := range targets {
				installed, ver, err := a.Database.IsInstalled(name)
				if err != nil {
					return err
				}
				if !installed {
					fmt.Printf("%s: not installed, skipping\n", name)
					continue
				}
				pkg, ok := a.Index.Get(name)
				if !ok {
					fmt.Printf("%s: not in index\n", name)
					continue
				}
				if pkg.Version == ver {
					fmt.Printf("%s: already at %s\n", name, ver)
					continue
				}
				// v1: full resumable download when no local prior bottle for binary diff
				if err := upgradePackage(ctx, a, pkg); err != nil {
					return err
				}
				fmt.Printf("%s: upgraded %s -> %s\n", name, ver, pkg.Version)
			}
			return nil
		},
	}
}

func upgradePackage(ctx context.Context, a *app.App, pkg index.Package) error {
	dest := filepath.Join(a.DownloadsDir, pkg.Bottle.SHA256+".bottle.tar.gz")
	spec := download.DownloadSpec{
		ID:         pkg.Name,
		URL:        pkg.Bottle.URL,
		Dest:       dest,
		FileSHA256: pkg.Bottle.SHA256,
		MaxWorkers: a.Jobs,
	}
	if err := a.Downloader.Download(ctx, spec, nil); err != nil {
		return err
	}
	_ = a.Database.RemoveInstall(pkg.Name)
	return a.Installer.InstallNode(ctx, resolver.Node{Name: pkg.Name, Package: pkg})
}
