// Package collector assembles the metadata collector and owns its resources.
package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type App struct {
	Logger    *slog.Logger
	Metadata  *metadata.Service
	Favorites *favorites.Service
	APIToken  string

	db    *sql.DB
	feeds *feed.Service
}

func New(ctx context.Context, logger *slog.Logger) (*App, error) {
	token := strings.TrimSpace(os.Getenv("TANA_COLLECTOR_API_TOKEN"))
	if token == "" || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return nil, fmt.Errorf("TANA_COLLECTOR_API_TOKEN must be nonempty and contain no whitespace")
	}
	cfg, err := feed.LoadConfig(os.Getenv)
	if err != nil {
		return nil, err
	}
	pandaCfg, err := panda.LoadConfig(os.Getenv)
	if err != nil {
		return nil, err
	}
	// Collection uses sustained pacing even if other Panda consumers allow bursts.
	favoritesCfg, err := panda.LoadFavoritesConfig(os.Getenv)
	if err != nil {
		return nil, err
	}
	limiter, err := panda.NewRateLimiter(pandaCfg.RateInterval, 1)
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
	ban := pandaban.New(db)
	client, err := panda.NewClient(pandaCfg.APIURL, &http.Client{
		Timeout: time.Minute, Transport: panda.RateLimitedTransport(limiter, panda.BanTransport(ban, nil)),
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	favoritesLimiter, err := panda.NewRateLimiter(panda.FavoritesRateInterval, 1)
	if err != nil {
		db.Close()
		return nil, err
	}
	favoritesClient, err := panda.NewFavoritesClient(favoritesCfg, &http.Client{
		Timeout: time.Minute, Transport: panda.RateLimitedTransport(favoritesLimiter, panda.BanTransport(ban, nil)),
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &App{
		Logger:    logger,
		APIToken:  token,
		db:        db,
		feeds:     feed.New(ctx, db, cfg, nil, logger),
		Metadata:  metadata.New(ctx, db, client, logger),
		Favorites: favorites.New(ctx, db, favoritesCfg, favoritesClient, logger),
	}, nil
}

// Close stops background work before releasing its database.
func (a *App) Close() error {
	a.Favorites.Close()
	a.feeds.Close()
	a.Metadata.Close()
	return a.db.Close()
}
