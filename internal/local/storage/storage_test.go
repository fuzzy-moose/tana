package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabasePersistsAndMigratesOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data #? with spaces")
	db, _, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	var id int64
	err = db.QueryRow("INSERT INTO libraries (name, path) VALUES (?, ?) RETURNING id", "Persistent", t.TempDir()).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	if id != 1 {
		t.Fatalf("first library ID: %d", id)
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
	if err := db.QueryRow("SELECT name FROM libraries WHERE id = ?", id).Scan(&name); err != nil || name != "Persistent" {
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
