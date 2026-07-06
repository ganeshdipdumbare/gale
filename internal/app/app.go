package app

import (
	"fmt"
	"os"
	"runtime"

	"github.com/ganeshdipdumbare/gale/internal/db"
	"github.com/ganeshdipdumbare/gale/internal/download"
	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/ganeshdipdumbare/gale/internal/paths"
	"github.com/ganeshdipdumbare/gale/internal/resolver"
	"github.com/ganeshdipdumbare/gale/internal/store"
)

// App holds shared gale runtime state.
type App struct {
	NoTUI   bool
	Jobs    int
	Verbose bool

	IndexClient *index.Client
	Index       *index.Index
	Database    *db.DB
	Store       *store.Store
	Downloader  *download.Downloader
	Installer   *resolver.Installer

	DownloadsDir string
	OptDir       string
	BinDir       string
}

// Open initializes gale directories and services.
func Open(noTUI bool, jobs int, verbose bool) (*App, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	dbPath, err := paths.DB()
	if err != nil {
		return nil, err
	}
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	storeDir, err := paths.Store()
	if err != nil {
		database.Close()
		return nil, err
	}
	indexDir, err := paths.Index()
	if err != nil {
		database.Close()
		return nil, err
	}
	dlDir, err := paths.Downloads()
	if err != nil {
		database.Close()
		return nil, err
	}
	optDir, err := paths.Opt()
	if err != nil {
		database.Close()
		return nil, err
	}
	binDir, err := paths.Bin()
	if err != nil {
		database.Close()
		return nil, err
	}

	client := index.NewClient(indexDir)
	idx, _ := client.Load()

	a := &App{
		NoTUI:        noTUI,
		Jobs:         jobs,
		Verbose:      verbose,
		IndexClient:  client,
		Index:        idx,
		Database:     database,
		Store:        store.New(storeDir),
		Downloader:   download.NewDownloader(),
		DownloadsDir: dlDir,
		OptDir:       optDir,
		BinDir:       binDir,
	}
	if jobs <= 0 {
		a.Jobs = runtime.NumCPU()
		if a.Jobs < 4 {
			a.Jobs = 4
		}
	}
	a.Installer = &resolver.Installer{
		Downloader: a.Downloader,
		Store:      a.Store,
		DB:         a.Database,
		Downloads:  a.DownloadsDir,
		Opt:        a.OptDir,
		Bin:        a.BinDir,
		MaxJobs:    a.Jobs,
	}
	return a, nil
}

func (a *App) Close() error {
	if a.Database != nil {
		return a.Database.Close()
	}
	return nil
}

// EnsureIndex loads or updates the package index.
func (a *App) EnsureIndex(force bool) error {
	if a.Index != nil && !force {
		return nil
	}
	idx, _, err := a.IndexClient.Update(force)
	if err != nil {
		if a.Index != nil {
			return nil // use stale cache
		}
		return err
	}
	a.Index = idx
	return nil
}

func (a *App) Logf(format string, args ...any) {
	if a.Verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}
