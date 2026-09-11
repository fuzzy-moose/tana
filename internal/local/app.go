// Package local assembles the local application and owns its resources.
package local

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log/slog"
	"os"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/enrichment"
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
	Collector *collectorapi.Client

	db         *sql.DB
	enrichment *enrichment.Service
}

// New opens application storage and starts background availability checks.
func New(ctx context.Context, cfg Config, logger *slog.Logger) (*App, error) {
	var collectorClient *collectorapi.Client
	if cfg.CollectorURL != "" {
		var err error
		collectorClient, err = collectorapi.NewClient(cfg.CollectorURL, cfg.CollectorAPIToken)
		if err != nil {
			return nil, fmt.Errorf("configure Panda enrichment: %w", err)
		}
	}
	var web fs.FS
	if cfg.WebDir != "" {
		web = os.DirFS(cfg.WebDir)
		index, err := fs.Stat(web, "index.html")
		if err != nil {
			return nil, fmt.Errorf("open web UI: %w", err)
		}
		if index.IsDir() {
			return nil, fmt.Errorf("web UI index.html must be a file")
		}
	}
	db, dataDir, err := storage.Open(ctx, cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("open local storage: %w", err)
	}
	libraries, err := library.New(ctx, library.NewSQLiteRepository(db), os.DirFS, dataDir, logger)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open libraries: %w", err)
	}
	var enrich *enrichment.Service
	if collectorClient != nil {
		enrich = enrichment.New(ctx, db, collectorClient, logger)
	}
	scans := scan.New(ctx, db, libraries, os.DirFS, logger, enrich)
	return &App{
		Logger:     logger,
		Libraries:  libraries,
		Scans:      scans,
		Galleries:  gallery.NewSQLiteRepository(db),
		Web:        web,
		Collector:  collectorClient,
		db:         db,
		enrichment: enrich,
	}, nil
}

// Close stops background work and releases storage after HTTP requests drain.
func (a *App) Close() error {
	a.Scans.Close()
	if a.enrichment != nil {
		a.enrichment.Close()
	}
	a.Libraries.Close()
	return a.db.Close()
}
