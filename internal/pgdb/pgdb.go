// Package pgdb opens PostgreSQL connection pools and migrates their schema.
package pgdb

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLock is the pg_advisory_xact_lock key held while migrating.
// Advisory locks are scoped to the current database and each service owns its
// database, so one fixed key serialises migrations per service.
const migrationLock int64 = 0x74616e612d6d6967 // "tana-mig"

// Option adjusts the pool configuration parsed from the URL.
type Option func(*pgxpool.Config)

// WithMaxConns caps the pool size; values below one keep the URL's
// pool_max_conns or the pgx default.
func WithMaxConns(n int32) Option {
	return func(c *pgxpool.Config) {
		if n > 0 {
			c.MaxConns = n
		}
	}
}

// Open connects to url and applies the NNNN_name.sql files found at the root of
// migrations in filename order. Callers embedding migrations/*.sql pass
// fs.Sub(embedded, "migrations"). The caller owns the pool and must close it.
func Open(ctx context.Context, url string, migrations fs.FS, opts ...Option) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	for _, opt := range opts {
		opt(cfg)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := migrate(ctx, pool, migrations); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS) error {
	names, err := fs.Glob(migrations, "*.sql")
	if err != nil {
		return err
	}
	for i, name := range names {
		if !strings.HasPrefix(name, fmt.Sprintf("%04d_", i+1)) {
			return fmt.Errorf("migration %s: expected version %04d", name, i+1)
		}
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLock); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return err
	}
	var version int
	if err := tx.QueryRow(ctx, "SELECT coalesce(max(version), 0) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version > len(names) {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, len(names))
	}
	for i := version; i < len(names); i++ {
		ddl, err := fs.ReadFile(migrations, names[i])
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(ddl)); err != nil {
			return fmt.Errorf("migration %s: %w", names[i], err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", i+1); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
