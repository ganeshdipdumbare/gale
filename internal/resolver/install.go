package resolver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ganeshdipdumbare/gale/internal/db"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/store"
	"golang.org/x/sync/errgroup"
)

// Installer coordinates parallel package installation.
type Installer struct {
	Downloader *download.Downloader
	Store      *store.Store
	DB         *db.DB
	Downloads  string
	Opt        string
	Bin        string
	MaxJobs    int
	Progress   chan<- download.Event
	OnState    func(pkg string, state download.State)
}

// InstallNode installs a single resolved node.
func (ins *Installer) InstallNode(ctx context.Context, node Node) error {
	if ins.OnState != nil {
		ins.OnState(node.Name, download.StateQueued)
	}
	installed, _, err := ins.DB.IsInstalled(node.Name)
	if err != nil {
		return err
	}
	if installed {
		if ins.OnState != nil {
			ins.OnState(node.Name, download.StateDone)
		}
		return nil
	}

	pkg := node.Package
	if !pkg.HasBottle() {
		return fmt.Errorf("%s: no bottle available for this platform", node.Name)
	}
	dest := filepath.Join(ins.Downloads, pkg.Bottle.SHA256+".bottle.tar.gz")
	needDownload := true
	if _, err := os.Stat(dest); err == nil {
		if err := store.VerifyFileSHA256(dest, pkg.Bottle.SHA256); err == nil {
			needDownload = false
		}
	}
	if needDownload {
		spec := download.DownloadSpec{
			ID:         node.Name,
			URL:        pkg.Bottle.URL,
			Dest:       dest,
			FileSHA256: pkg.Bottle.SHA256,
			MaxWorkers: ins.MaxJobs,
		}
		if ins.OnState != nil {
			ins.OnState(node.Name, download.StateDownloading)
		}
		if err := ins.Downloader.Download(ctx, spec, ins.Progress); err != nil {
			if ins.OnState != nil {
				ins.OnState(node.Name, download.StateFailed)
			}
			return fmt.Errorf("%s: download: %w", node.Name, err)
		}
	}

	if ins.OnState != nil {
		ins.OnState(node.Name, download.StateInstalling)
	}
	storePath, err := ins.Store.IngestBottle(dest, pkg.Bottle.SHA256)
	if err != nil {
		return fmt.Errorf("%s: ingest: %w", node.Name, err)
	}
	target, err := ins.Store.LinkIntoPrefix(storePath, ins.Opt, node.Name, pkg.Version)
	if err != nil {
		return fmt.Errorf("%s: link: %w", node.Name, err)
	}
	if err := ins.Store.SymlinkBin(ins.Opt, ins.Bin, node.Name, pkg.Version); err != nil {
		return fmt.Errorf("%s: symlink: %w", node.Name, err)
	}

	pkgID, err := ins.DB.UpsertPackage(node.Name, pkg.Description)
	if err != nil {
		return err
	}
	verID, err := ins.DB.UpsertVersion(pkgID, pkg.Version, pkg.Bottle.SHA256, pkg.Bottle.URL, pkg.Bottle.Size)
	if err != nil {
		return err
	}
	_ = ins.DB.SetDependencies(pkgID, pkg.Dependencies)
	if err := ins.DB.RecordInstall(pkgID, verID, target); err != nil {
		return err
	}
	_ = ins.DB.RecordFile(pkg.Bottle.SHA256, storePath, pkg.Bottle.Size)
	if ins.OnState != nil {
		ins.OnState(node.Name, download.StateDone)
	}
	return nil
}

// InstallCoordinator runs parallel installs level by level.
type InstallCoordinator struct {
	Installer *Installer
}

func (c *InstallCoordinator) Run(ctx context.Context, g *Graph) error {
	ins := c.Installer
	levels := g.Levels()
	maxJobs := ins.MaxJobs
	if maxJobs <= 0 {
		maxJobs = 4
	}
	sem := make(chan struct{}, maxJobs)

	for _, level := range levels {
		eg, ctx := errgroup.WithContext(ctx)
		for _, name := range level {
			name := name
			node := g.Nodes[name]
			eg.Go(func() error {
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
					return ctx.Err()
				}
				return ins.InstallNode(ctx, node)
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
	}
	return nil
}
