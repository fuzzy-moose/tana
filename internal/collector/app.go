// Package collector assembles the metadata collector and owns its resources.
package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type App struct {
	db       *sql.DB
	feeds    *feed.Service
	metadata *metadata.Service
	handler  http.Handler
}

func New(ctx context.Context, logger *slog.Logger) (*App, error) {
	cfg, err := feed.LoadConfig(os.Getenv)
	if err != nil {
		return nil, err
	}
	pandaCfg, err := panda.LoadConfig(os.Getenv)
	if err != nil {
		return nil, err
	}
	// Collection uses sustained pacing even if other Panda consumers allow bursts.
	limiter, err := panda.NewRateLimiter(pandaCfg.RateInterval, 1)
	if err != nil {
		return nil, err
	}
	client, err := panda.NewClient(pandaCfg.APIURL, &http.Client{
		Timeout: time.Minute, Transport: panda.RateLimitedTransport(limiter, nil),
	})
	if err != nil {
		return nil, err
	}
	dir, err := storage.DataDir()
	if err != nil {
		return nil, fmt.Errorf("resolve collector storage: %w", err)
	}
	db, _, err := storage.Open(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("open collector storage: %w", err)
	}
	return &App{
		db: db, feeds: feed.New(ctx, db, cfg, nil, logger), handler: httpapi.NewHandler(logger),
		metadata: metadata.New(ctx, db, client, logger),
	}, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}

// Close stops background work before releasing its database.
func (a *App) Close() error {
	a.feeds.Close()
	a.metadata.Close()
	return a.db.Close()
}
