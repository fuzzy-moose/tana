// Package local assembles the local application and owns its resources.
package local

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/fuzzy-moose/tana/internal/local/httpapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/scan"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

type App struct {
	db        *sql.DB
	libraries *library.Service
	scans     *scan.Service
	handler   http.Handler
}

// New opens application storage and starts background availability checks.
func New(ctx context.Context, logger *slog.Logger) (*App, error) {
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
		db:        db,
		libraries: libraries,
		scans:     scans,
		handler:   httpapi.NewHandler(logger, libraries, scans),
	}, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}

// Close stops background work and releases storage after HTTP requests drain.
func (a *App) Close() error {
	a.scans.Close()
	a.libraries.Close()
	return a.db.Close()
}
