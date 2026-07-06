package store

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// extractTarGz unpacks a bottle; uses the system tar on Unix for speed.
func extractTarGz(src, dest string) error {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		if err := extractTarGzSystem(src, dest); err == nil {
			return nil
		}
	}
	return extractTarGzGo(src, dest)
}

func extractTarGzSystem(src, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("tar", "-xzf", src, "-C", dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %w: %s", err, out)
	}
	return nil
}
