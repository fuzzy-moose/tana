package library

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDatabasePersistsAndMigratesOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data #? with spaces")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s, err := openService(t.Context(), dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := s.Create(t.Context(), "Persistent", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = openService(t.Context(), dir, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	row, err := s.Get(t.Context(), registered.ID)
	if err != nil || row.Name != registered.Name || row.Path != registered.Path {
		t.Fatalf("registration did not survive reopen: %+v %v", row, err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("schema version %d: %v", version, err)
	}
	// Force a fresh pooled connection: connection-local settings must survive.
	s.db.SetConnMaxLifetime(time.Nanosecond)
	for _, tc := range []struct {
		pragma string
		want   int
	}{{"foreign_keys", 1}, {"busy_timeout", 5000}} {
		var value int
		if err := s.db.QueryRow("PRAGMA " + tc.pragma).Scan(&value); err != nil || value != tc.want {
			t.Fatalf("%s=%d, error=%v", tc.pragma, value, err)
		}
	}
	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode=%q, error=%v", mode, err)
	}
}

func TestLibraryDeletionCascadesToOwnedRecords(t *testing.T) {
	s := testService(t)
	root, err := s.Create(t.Context(), "Comics", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the relationship required of future catalog/progress tables.
	_, err = s.db.Exec(`CREATE TABLE owned_records (
		library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
		value TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("INSERT INTO owned_records VALUES (?, 'progress')", root.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), root.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM owned_records").Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned records=%d: %v", count, err)
	}
	if _, err := s.db.Exec("INSERT INTO owned_records VALUES ('missing', 'progress')"); err == nil {
		t.Fatal("foreign key constraint not enforced")
	}
}

func TestFutureDatabaseVersionRejected(t *testing.T) {
	s := testService(t)
	if _, err := s.db.Exec("PRAGMA user_version = 999"); err != nil {
		t.Fatal(err)
	}
	if err := migrate(t.Context(), s.db); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("future schema accepted: %v", err)
	}
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 999 {
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
