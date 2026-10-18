// Package storage opens and migrates the collector database.
package storage

import (
	"context"
	"embed"
	"io/fs"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/pgdb"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate -f ../../../sqlc.yaml

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the collector schema migrations laid out for pgdb.Open.
func Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

// Open connects to the collector database at url and migrates it. The caller
// owns the pool and must close it after its users have stopped.
func Open(ctx context.Context, url string, maxConns int32) (*pgxpool.Pool, error) {
	return pgdb.Open(ctx, url, Migrations(), pgdb.WithMaxConns(maxConns))
}
