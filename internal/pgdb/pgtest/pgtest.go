// Package pgtest gives each test its own migrated PostgreSQL database.
package pgtest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// EnvURL names an existing server to use instead of a container. Its role
// needs CREATEDB; databases are created next to the one in the URL.
const EnvURL = "TANA_TEST_DATABASE_URL"

type server struct {
	url   *url.URL
	admin *pgxpool.Pool
}

var (
	getServer   = sync.OnceValues(startServer)
	templatesMu sync.Mutex
)

// Open returns a pool on a fresh database cloned from a template migrated
// with migrations (laid out as for pgdb.Open). The database is dropped when
// the test ends.
func Open(t testing.TB, migrations fs.FS) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	srv, err := getServer()
	if err != nil {
		t.Fatalf("pgtest: no PostgreSQL server (needs Docker, or %s pointing at a server whose role has CREATEDB): %v", EnvURL, err)
	}
	tpl, err := srv.ensureTemplate(ctx, migrations)
	if err != nil {
		t.Fatalf("pgtest: template database: %v", err)
	}
	name := "pgtest_" + randomName()
	if _, err := srv.admin.Exec(ctx, "CREATE DATABASE "+ident(name)+" TEMPLATE "+ident(tpl)); err != nil {
		t.Fatalf("pgtest: create database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := srv.admin.Exec(context.Background(), "DROP DATABASE "+ident(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("pgtest: drop database %s: %v", name, err)
		}
	})
	pool, err := pgdb.Open(ctx, srv.databaseURL(name), migrations)
	if err != nil {
		t.Fatalf("pgtest: open database: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Begin starts a transaction on pool that is rolled back when the test ends,
// before the pool closes.
func Begin(t testing.TB, pool *pgxpool.Pool) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pgtest: begin: %v", err)
	}
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	return tx
}

// Reopen closes pool and returns a new pool on the same database, opened and
// migrated as a restarted service would open it.
func Reopen(t testing.TB, pool *pgxpool.Pool, migrations fs.FS) *pgxpool.Pool {
	t.Helper()
	url := pool.Config().ConnString()
	pool.Close()
	reopened, err := pgdb.Open(context.Background(), url, migrations)
	if err != nil {
		t.Fatalf("pgtest: reopen database: %v", err)
	}
	t.Cleanup(reopened.Close)
	return reopened
}

// Synctest runs f in a synctest bubble with a pool on a fresh database. pgx
// connections must not cross the bubble boundary, so the database is created
// outside the bubble and its pool is opened and closed inside it.
func Synctest(t *testing.T, migrations fs.FS, f func(t *testing.T, pool *pgxpool.Pool)) {
	t.Helper()
	outside := Open(t, migrations)
	url := outside.Config().ConnString()
	outside.Close()
	synctest.Test(t, func(t *testing.T) {
		pool, err := pgdb.Open(context.Background(), url, migrations)
		if err != nil {
			t.Fatalf("pgtest: open database: %v", err)
		}
		t.Cleanup(pool.Close)
		f(t, pool)
	})
}

func startServer() (*server, error) {
	ctx := context.Background()
	raw := os.Getenv(EnvURL)
	if raw == "" {
		// The testcontainers reaper removes the container after the test binary exits.
		c, err := postgres.Run(ctx, "postgres:18-alpine", postgres.BasicWaitStrategies())
		if err != nil {
			return nil, err
		}
		if raw, err = c.ConnectionString(ctx, "sslmode=disable"); err != nil {
			return nil, err
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	pool, err := pgxpool.New(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &server{url: u, admin: pool}, nil
}

// ensureTemplate returns the name of a database migrated with migrations,
// keyed by their content so distinct migration sets get distinct templates.
func (s *server) ensureTemplate(ctx context.Context, migrations fs.FS) (string, error) {
	sum, err := digest(migrations)
	if err != nil {
		return "", err
	}
	name := "pgtest_tpl_" + sum
	templatesMu.Lock()
	defer templatesMu.Unlock()
	var exists bool
	if err := s.admin.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", name).Scan(&exists); err != nil {
		return "", err
	}
	if !exists {
		if err := s.buildTemplate(ctx, name, migrations); err != nil {
			return "", err
		}
	}
	return name, nil
}

// buildTemplate migrates under a temporary name and renames it into place, so
// test binaries sharing a TANA_TEST_DATABASE_URL server never clone a template
// that is still being migrated. A template left by an earlier run is reused:
// its name pins the migration contents.
func (s *server) buildTemplate(ctx context.Context, name string, migrations fs.FS) error {
	tmp := "pgtest_build_" + randomName()
	if _, err := s.admin.Exec(ctx, "CREATE DATABASE "+ident(tmp)); err != nil {
		return err
	}
	defer s.admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(tmp)+" WITH (FORCE)")
	pool, err := pgdb.Open(ctx, s.databaseURL(tmp), migrations)
	if err != nil {
		return err
	}
	pool.Close()
	_, err = s.admin.Exec(ctx, "ALTER DATABASE "+ident(tmp)+" RENAME TO "+ident(name))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P04" { // duplicate_database: another binary won.
		return nil
	}
	return err
}

func (s *server) databaseURL(name string) string {
	u := *s.url
	u.Path = "/" + name
	u.RawPath = ""
	return u.String()
}

func digest(migrations fs.FS) (string, error) {
	names, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, name := range names {
		body, err := fs.ReadFile(migrations, name)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00", name, len(body))
		h.Write(body)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

func ident(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

func randomName() string {
	return strings.ToLower(rand.Text())
}
