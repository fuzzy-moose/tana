// Package storage opens and migrates the local application database.
package storage

import (
	"context"
	"embed"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/pgdb"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate -f ../../../sqlc.yaml

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the local schema migrations laid out for pgdb.Open.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

// Open connects to the local database at url and migrates it. The caller owns
// the pool and must close it after its users have stopped.
func Open(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	return pgdb.Open(ctx, url, Migrations(), pgdb.WithMaxConns(maxConns))
}

// LockCatalog blocks writes to libraries, sources, source files, galleries and
// gallery pages until tx ends. Deletions that revalidate a reviewed catalog
// state take it first, so the state cannot change between revalidation and
// deletion: row locks cannot stop the inserts that revalidation depends on.
// SHARE ROW EXCLUSIVE conflicts with itself, so two removals queue, while
// readers and foreign-key checks proceed.
func LockCatalog(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "LOCK TABLE libraries, sources, source_files, galleries, gallery_pages IN SHARE ROW EXCLUSIVE MODE")
	return err
}
