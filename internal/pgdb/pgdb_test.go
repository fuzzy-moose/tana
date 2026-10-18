package pgdb_test

import (
	"embed"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/pgdb"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

//go:embed testdata/migrations/*.sql
var embedded embed.FS

func testMigrations(t *testing.T) fs.FS {
	t.Helper()
	migrations, err := fs.Sub(embedded, "testdata/migrations")
	if err != nil {
		t.Fatal(err)
	}
	return migrations
}

func reopen(t *testing.T, pool *pgxpool.Pool, migrations fs.FS) (*pgxpool.Pool, error) {
	t.Helper()
	reopened, err := pgdb.Open(t.Context(), pool.Config().ConnString(), migrations)
	if err == nil {
		t.Cleanup(reopened.Close)
	}
	return reopened, err
}

func TestOpenMigratesEmptyDatabase(t *testing.T) {
	empty := pgtest.Open(t, fstest.MapFS{})
	migrations := testMigrations(t)
	// Concurrent opens serialise on the advisory lock; each version applies once.
	errs := make(chan error, 3)
	for range cap(errs) {
		go func() {
			_, err := reopen(t, empty, migrations)
			errs <- err
		}()
	}
	for range cap(errs) {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var versions string
	if err := empty.QueryRow(t.Context(), "SELECT string_agg(version::text, ',' ORDER BY version) FROM schema_migrations").Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != "1,2" {
		t.Fatalf("versions = %q", versions)
	}
	if _, err := empty.Exec(t.Context(), "INSERT INTO tags VALUES ('a'); INSERT INTO items (name, tag) VALUES ('x', 'a')"); err != nil {
		t.Fatal(err)
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	migrations := testMigrations(t)
	pool := pgtest.Open(t, migrations)
	if _, err := pool.Exec(t.Context(), "INSERT INTO items (name) VALUES ('kept')"); err != nil {
		t.Fatal(err)
	}
	var before time.Time
	if err := pool.QueryRow(t.Context(), "SELECT max(applied_at) FROM schema_migrations").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := reopen(t, pool, migrations); err != nil {
		t.Fatal(err)
	}
	var count, items int
	var after time.Time
	if err := pool.QueryRow(t.Context(), `SELECT count(*), max(applied_at), (SELECT count(*) FROM items)
		FROM schema_migrations`).Scan(&count, &after, &items); err != nil {
		t.Fatal(err)
	}
	if count != 2 || !after.Equal(before) || items != 1 {
		t.Fatalf("count=%d applied_at %v -> %v items=%d", count, before, after, items)
	}
}

func TestOpenRefusesNewerDatabase(t *testing.T) {
	migrations := testMigrations(t)
	pool := pgtest.Open(t, migrations)
	if _, err := pool.Exec(t.Context(), "INSERT INTO schema_migrations (version) VALUES (3)"); err != nil {
		t.Fatal(err)
	}
	_, err := reopen(t, pool, migrations)
	if err == nil || !strings.Contains(err.Error(), "database schema version 3 is newer than supported version 2") {
		t.Fatalf("err = %v", err)
	}
}
