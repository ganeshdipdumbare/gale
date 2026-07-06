package paths

import (
	"os"
	"path/filepath"
)

// Root returns ~/.gale (or $GALE_HOME if set).
func Root() (string, error) {
	if home := os.Getenv("GALE_HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gale"), nil
}

func Store() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "store"), nil
}

func Index() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "index"), nil
}

func DB() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "gale.db"), nil
}

func Opt() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "opt"), nil
}

func Bin() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "bin"), nil
}

func Downloads() (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "downloads"), nil
}

// Ensure creates gale data directories.
func Ensure() error {
	dirs := []func() (string, error){Store, Index, Opt, Bin, Downloads}
	for _, fn := range dirs {
		p, err := fn()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}
	return nil
}
