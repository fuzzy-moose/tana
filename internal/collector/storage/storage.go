// Package storage opens and migrates the collector database.
package storage

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/fuzzy-moose/tana/internal/panda"

	_ "modernc.org/sqlite"
)

//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate -f ../../../sqlc.yaml

//go:embed migrations/*.sql
var migrations embed.FS

// Open returns the initialized database and absolute application storage path.
// The caller owns the database and must close it after its users have stopped.
func Open(ctx context.Context, dir string) (*sql.DB, string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, "", fmt.Errorf("create application storage: %w", err)
	}
	path := filepath.ToSlash(filepath.Join(dir, "collector.db"))
	if !filepath.IsAbs(path) || path[0] != '/' {
		path = "/" + path // file:///C:/... on Windows.
	}
	uri := url.URL{Scheme: "file", Path: path}
	params := url.Values{
		"_pragma": {"foreign_keys(1)", "busy_timeout(5000)"},
		"_txlock": {"immediate"},
	}
	uri.RawQuery = params.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, "", err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	fail := func(err error) (*sql.DB, string, error) {
		_ = db.Close()
		return nil, "", err
	}
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&mode); err != nil {
		return fail(fmt.Errorf("enable SQLite WAL: %w", err))
	}
	if mode != "wal" {
		return fail(fmt.Errorf("expected SQLite WAL, got %q", mode))
	}
	if err := migrate(ctx, db); err != nil {
		return fail(err)
	}
	return db, dir, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(entries) {
		return fmt.Errorf("database schema version %d is newer than supported version %d", version, len(entries))
	}
	for i := version; i < len(entries); i++ {
		ddl, err := migrations.ReadFile("migrations/" + entries[i].Name())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(ddl)); err != nil {
			return fmt.Errorf("migration %s: %w", entries[i].Name(), err)
		}
		if entries[i].Name() == "0002_feed_gallery_refs.sql" {
			if err := backfillFeedSightings(ctx, tx); err != nil {
				return fmt.Errorf("backfill feed sightings: %w", err)
			}
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func backfillFeedSightings(ctx context.Context, tx *sql.Tx) error {
	// Parse one retained capture at a time; the complete feed history can be large.
	var after int64
	for {
		var id int64
		var body []byte
		err := tx.QueryRowContext(ctx, `SELECT id, body FROM raw_feeds
			WHERE processed_at IS NOT NULL AND id > ? ORDER BY id LIMIT 1`, after).Scan(&id, &body)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		entries, err := panda.ParseFeed(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("capture %d: %w", id, err)
		}
		for _, entry := range entries {
			if _, err := tx.ExecContext(ctx, `INSERT INTO feed_gallery_refs (raw_feed_id, gallery_id)
				VALUES (?, ?) ON CONFLICT DO NOTHING`, id, entry.GalleryRef.ID); err != nil {
				return err
			}
		}
		after = id
	}
}
