// Package local assembles the local application and owns its resources.
package local

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/scan"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

type App struct {
	Logger    *slog.Logger
	Libraries *library.Service
	Scans     *scan.Service
	Galleries *gallery.SQLiteRepository
	Web       fs.FS

	db *sql.DB
}

// New opens application storage and starts background availability checks.
func New(ctx context.Context, logger *slog.Logger) (*App, error) {
	var web fs.FS
	if dir := strings.TrimSpace(os.Getenv("TANA_WEB_DIR")); dir != "" {
		web = os.DirFS(dir)
		index, err := fs.Stat(web, "index.html")
		if err != nil {
			return nil, fmt.Errorf("open web UI: %w", err)
		}
		if index.IsDir() {
			return nil, fmt.Errorf("web UI index.html must be a file")
		}
	}
	dataDir, err := storage.DataDir()
	if err != nil {
		return nil, fmt.Errorf("resolve local storage: %w", err)
	}
	db, dataDir, err := storage.Open(ctx, dataDir)
	if err != nil {
		return nil, fmt.Errorf("open local storage: %w", err)
	}
	libraries, err := library.New(ctx, library.NewSQLiteRepository(db), os.DirFS, dataDir, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open libraries: %w", err)
	}
	scans := scan.New(ctx, db, libraries, os.DirFS, logger)
	return &App{
		Logger:    logger,
		Libraries: libraries,
		Scans:     scans,
		Galleries: gallery.NewSQLiteRepository(db),
		Web:       web,
		db:        db,
	}, nil
}

// Close stops background work and releases storage after HTTP requests drain.
func (a *App) Close() error {
	a.Scans.Close()
	a.Libraries.Close()
	return a.db.Close()
}
