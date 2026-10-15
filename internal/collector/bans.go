package collector

import (
	"context"
	"errors"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/metadataproxy"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/sitemap"
)

var ErrUnknownBanGroup = errors.New("unknown Panda ban group")

// Bans lifts a request group's ban on explicit request. Work that recorded the
// ban as its retry deadline resumes too; other failures keep their backoff.
type Bans struct {
	main, authenticated *pandaban.State
	metadata            *metadata.Service
	sitemap             *sitemap.Service
	proxies             *metadataproxy.Service
	favorites           *favorites.Service
	downloads           *downloads.Service
}

// Reset accepts "main", "authenticated", or "proxies". A ban still in force
// upstream is recorded again by the next request through that group.
func (b *Bans) Reset(ctx context.Context, group string) error {
	switch group {
	case "main":
		if err := b.main.Reset(ctx); err != nil {
			return err
		}
		if err := b.metadata.ClearBanRetry(ctx); err != nil {
			return err
		}
		return b.sitemap.ClearBanRetries(ctx)
	case "authenticated":
		if err := b.authenticated.Reset(ctx); err != nil {
			return err
		}
		if err := b.favorites.ClearBanRetries(ctx); err != nil {
			return err
		}
		return b.downloads.ClearBanRetries(ctx)
	case "proxies":
		return b.proxies.ResetBans(ctx)
	}
	return ErrUnknownBanGroup
}
