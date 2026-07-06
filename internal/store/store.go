package store

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/gzip"

	"golang.org/x/sys/unix"
)

// Store is a content-addressed package store.
type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) ObjectPath(sha256sum string) string {
	return filepath.Join(s.root, sha256sum)
}

// Has returns true if an object exists in the store.
func (s *Store) Has(sha256sum string) bool {
	_, err := os.Stat(s.ObjectPath(sha256sum))
	return err == nil
}

// IngestBottle extracts a verified bottle tarball into the content-addressed store.
func (s *Store) IngestBottle(bottlePath, sha256sum string) (string, error) {
	if s.Has(sha256sum) {
		return s.ObjectPath(sha256sum), nil
	}
	dest := s.ObjectPath(sha256sum)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}
	if err := extractTarGz(bottlePath, dest); err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	return dest, nil
}

// LinkIntoPrefix materializes store content into the install prefix.
// Uses a symlink when the bottle layout matches (fast); falls back to APFS clonefile.
func (s *Store) LinkIntoPrefix(storePath, prefix, pkgName, version string) (string, error) {
	target := filepath.Join(prefix, pkgName, version)
	if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}

	// Homebrew bottles extract to <name>/<version>/ inside the store object.
	inner := filepath.Join(storePath, pkgName, version)
	if st, err := os.Stat(inner); err == nil && st.IsDir() {
		if err := os.Symlink(inner, target); err == nil {
			return target, nil
		}
	}
	if err := cloneOrCopyDir(storePath, target); err != nil {
		return "", err
	}
	return target, nil
}

// SymlinkBin creates symlinks for executables found under prefix into binDir.
func (s *Store) SymlinkBin(prefix, binDir, pkgName, version string) error {
	pkgRoot := filepath.Join(prefix, pkgName, version)
	return filepath.Walk(pkgRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(pkgRoot, path)
		if err != nil {
			return err
		}
		// Homebrew bottles place binaries under <version>/bin/
		if !strings.HasPrefix(rel, "bin/") && !strings.Contains(rel, "/bin/") {
			return nil
		}
		if info.Mode()&0o111 == 0 {
			return nil
		}
		base := filepath.Base(path)
		link := filepath.Join(binDir, base)
		_ = os.Remove(link)
		return os.Symlink(path, link)
	})
}

func extractTarGzGo(src, dest string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	buf := make([]byte, 256*1024)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.CopyBuffer(out, tr, buf); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}

func cloneOrCopyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := cloneFile(path, target); err != nil {
			return copyFile(path, target, info.Mode())
		}
		return nil
	})
}

func cloneFile(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err != nil {
		return err
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.CopyBuffer(out, in, make([]byte, 256*1024))
	return err
}

// VerifyFileSHA256 checks a file on disk.
func VerifyFileSHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("%s: checksum %s != %s", path, got, expected)
	}
	return nil
}

// DoctorIssue describes a store integrity problem.
type DoctorIssue struct {
	Kind    string
	Path    string
	Message string
}

// Doctor walks the store and reports issues.
func (s *Store) Doctor(fileRecords []struct {
	SHA256 string
	Path   string
	Size   int64
}) []DoctorIssue {
	var issues []DoctorIssue
	seen := map[string]bool{}
	for _, rec := range fileRecords {
		seen[rec.SHA256] = true
		p := s.ObjectPath(rec.SHA256)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			issues = append(issues, DoctorIssue{Kind: "missing", Path: p, Message: "object missing from store"})
			continue
		}
		if err := VerifyFileSHA256(p, rec.SHA256); err != nil {
			issues = append(issues, DoctorIssue{Kind: "checksum", Path: p, Message: err.Error()})
		}
	}
	entries, _ := os.ReadDir(s.root)
	for _, e := range entries {
		if !seen[e.Name()] {
			issues = append(issues, DoctorIssue{Kind: "orphan", Path: filepath.Join(s.root, e.Name()), Message: "unreferenced store object"})
		}
	}
	return issues
}
