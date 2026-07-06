package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite metadata store.
type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the gale metadata database.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS packages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT
);
CREATE TABLE IF NOT EXISTS versions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    package_id INTEGER NOT NULL REFERENCES packages(id),
    version TEXT NOT NULL,
    bottle_sha256 TEXT NOT NULL,
    bottle_url TEXT,
    install_size INTEGER DEFAULT 0,
    UNIQUE(package_id, version)
);
CREATE TABLE IF NOT EXISTS dependencies (
    package_id INTEGER NOT NULL REFERENCES packages(id),
    depends_on TEXT NOT NULL,
    PRIMARY KEY (package_id, depends_on)
);
CREATE TABLE IF NOT EXISTS installs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    package_id INTEGER NOT NULL REFERENCES packages(id),
    version_id INTEGER NOT NULL REFERENCES versions(id),
    prefix TEXT NOT NULL,
    installed_at TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'installed'
);
CREATE TABLE IF NOT EXISTS files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sha256 TEXT NOT NULL UNIQUE,
    path TEXT NOT NULL,
    size INTEGER NOT NULL
);
`
	_, err := d.conn.Exec(schema)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`)
	return err
}

// InstalledPackage represents a row for list output.
type InstalledPackage struct {
	Name        string
	Version     string
	InstalledAt time.Time
	Size        int64
	Status      string
}

func (d *DB) UpsertPackage(name, description string) (int64, error) {
	_, err := d.conn.Exec(`INSERT INTO packages(name, description) VALUES(?, ?)
		ON CONFLICT(name) DO UPDATE SET description=excluded.description`, name, description)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.conn.QueryRow(`SELECT id FROM packages WHERE name=?`, name).Scan(&id)
	return id, err
}

func (d *DB) UpsertVersion(packageID int64, version, sha256, url string, size int64) (int64, error) {
	_, err := d.conn.Exec(`INSERT INTO versions(package_id, version, bottle_sha256, bottle_url, install_size)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(package_id, version) DO UPDATE SET
			bottle_sha256=excluded.bottle_sha256,
			bottle_url=excluded.bottle_url,
			install_size=excluded.install_size`,
		packageID, version, sha256, url, size)
	if err != nil {
		return 0, err
	}
	var id int64
	err = d.conn.QueryRow(`SELECT id FROM versions WHERE package_id=? AND version=?`, packageID, version).Scan(&id)
	return id, err
}

func (d *DB) SetDependencies(packageID int64, deps []string) error {
	tx, err := d.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM dependencies WHERE package_id=?`, packageID); err != nil {
		return err
	}
	for _, dep := range deps {
		if _, err := tx.Exec(`INSERT INTO dependencies(package_id, depends_on) VALUES(?, ?)`, packageID, dep); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d *DB) RecordInstall(packageID, versionID int64, prefix string) error {
	_, err := d.conn.Exec(`INSERT INTO installs(package_id, version_id, prefix, installed_at, status)
		VALUES(?, ?, ?, ?, 'installed')`,
		packageID, versionID, prefix, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (d *DB) RecordFile(sha256, path string, size int64) error {
	_, err := d.conn.Exec(`INSERT INTO files(sha256, path, size) VALUES(?, ?, ?)
		ON CONFLICT(sha256) DO UPDATE SET path=excluded.path, size=excluded.size`,
		sha256, path, size)
	return err
}

func (d *DB) IsInstalled(name string) (bool, string, error) {
	var version string
	err := d.conn.QueryRow(`
		SELECT v.version FROM installs i
		JOIN versions v ON v.id = i.version_id
		JOIN packages p ON p.id = i.package_id
		WHERE p.name = ? AND i.status = 'installed'
		ORDER BY i.installed_at DESC LIMIT 1`, name).Scan(&version)
	if err == sql.ErrNoRows {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return true, version, nil
}

func (d *DB) ListInstalled() ([]InstalledPackage, error) {
	rows, err := d.conn.Query(`
		SELECT p.name, v.version, i.installed_at, v.install_size, i.status
		FROM installs i
		JOIN packages p ON p.id = i.package_id
		JOIN versions v ON v.id = i.version_id
		WHERE i.status = 'installed'
		ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstalledPackage
	for rows.Next() {
		var ip InstalledPackage
		var ts string
		if err := rows.Scan(&ip.Name, &ip.Version, &ts, &ip.Size, &ip.Status); err != nil {
			return nil, err
		}
		ip.InstalledAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, ip)
	}
	return out, rows.Err()
}

func (d *DB) RemoveInstall(name string) error {
	res, err := d.conn.Exec(`
		UPDATE installs SET status='removed'
		WHERE id IN (
			SELECT i.id FROM installs i
			JOIN packages p ON p.id = i.package_id
			WHERE p.name = ? AND i.status = 'installed'
		)`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("package %q not installed", name)
	}
	return nil
}

func (d *DB) AllFileRecords() ([]struct {
	SHA256 string
	Path   string
	Size   int64
}, error) {
	rows, err := d.conn.Query(`SELECT sha256, path, size FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []struct {
		SHA256 string
		Path   string
		Size   int64
	}
	for rows.Next() {
		var r struct {
			SHA256 string
			Path   string
			Size   int64
		}
		if err := rows.Scan(&r.SHA256, &r.Path, &r.Size); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) PackageID(name string) (int64, error) {
	var id int64
	err := d.conn.QueryRow(`SELECT id FROM packages WHERE name=?`, name).Scan(&id)
	return id, err
}
