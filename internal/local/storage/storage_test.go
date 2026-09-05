package storage

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestGalleryMigrationPreservesExistingLibrary(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "tana.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ddl, err := migrations.ReadFile("migrations/0001_libraries.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(ddl)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO libraries (id, name, path) VALUES ('existing', 'Existing', ?)", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRowContext(ctx, "SELECT name FROM libraries WHERE id = 'existing'").Scan(&name); err != nil || name != "Existing" {
		t.Fatalf("existing library: %q, %v", name, err)
	}
	if _, err := db.ExecContext(ctx, "INSERT INTO sources (id, library_id, path, kind) VALUES ('new-source', 'existing', 'book.zip', 'archive')"); err != nil {
		t.Fatalf("upgraded library cannot own a source: %v", err)
	}
}

func TestDatabasePersistsAndMigratesOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data #? with spaces")
	db, _, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO libraries (id, name, path) VALUES (?, ?, ?)", "persistent", "Persistent", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.QueryRow("SELECT name FROM libraries WHERE id = ?", "persistent").Scan(&name); err != nil || name != "Persistent" {
		t.Fatalf("registration did not survive reopen: %q %v", name, err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version %d: %v", version, err)
	}
}

func TestFutureDatabaseVersionRejected(t *testing.T) {
	db, _, err := Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), db); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future schema accepted: %v", err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 999 {
		t.Fatalf("future schema was changed: %d, %v", version, err)
	}
}

func TestDataDir(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	for _, tc := range []struct {
		name, goos string
		env        map[string]string
		want       string
	}{
		{"linux", "linux", nil, filepath.Join(home, ".local", "share", "tana")},
		{"xdg", "linux", map[string]string{"XDG_DATA_HOME": home}, filepath.Join(home, "tana")},
		{"relative xdg ignored", "linux", map[string]string{"XDG_DATA_HOME": "relative"}, filepath.Join(home, ".local", "share", "tana")},
		{"mac", "darwin", nil, filepath.Join(home, "Library", "Application Support", "tana")},
		{"windows", "windows", map[string]string{"LOCALAPPDATA": home}, filepath.Join(home, "tana")},
		{"override", "linux", map[string]string{"TANA_DATA_DIR": home}, home},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := dataDir(func(k string) string { return tc.env[k] }, func() (string, error) { return home, nil }, tc.goos)
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	_, err := dataDir(func(string) string { return "" }, func() (string, error) { return "", errors.New("no home") }, "linux")
	if err == nil {
		t.Fatal("missing home accepted")
	}
}
