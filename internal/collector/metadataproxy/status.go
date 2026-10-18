package metadataproxy

import (
	"context"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

func (s *Service) Status(ctx context.Context) (collectorapi.MetadataProxyStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, channels, err := s.channels(ctx)
	status := collectorapi.MetadataProxyStatus{Enabled: settings.Enabled, AutoRemoveInactive: settings.AutoRemoveInactive, Channels: []collectorapi.MetadataProxyChannel{},
		RateIntervalMS: s.config.RateInterval.Milliseconds(), DefaultUserAgent: defaultUserAgent}
	if err != nil {
		return status, err
	}
	now := time.Now()
	for _, ch := range channels {
		until, err := s.banUntil(ctx, ch.ID, ch.Endpoint)
		if err != nil {
			return status, err
		}
		item := collectorapi.MetadataProxyChannel{ID: ch.ID, Name: ch.Name, ProxyURL: ch.Endpoint, Username: ch.Username,
			HasPassword: ch.Password != "", UserAgent: ch.UserAgent, Enabled: ch.Enabled, State: "idle", LastError: ch.LastError}
		item.LastSuccessAt = pgdb.UTC(ch.LastSuccessAt)
		if ch.RetryAt != nil && ch.RetryAt.After(now) {
			item.RetryAt, item.State = pgdb.UTC(ch.RetryAt), "waiting_retry"
		}
		if runtime := s.runtime[ch.ID]; runtime != nil && runtime.holdUntil.After(pgdb.Latest(&now, ch.RetryAt)) {
			item.RetryAt, item.State = pgdb.UTC(&runtime.holdUntil), "waiting_retry"
		}
		if until.After(now) {
			item.BanUntil, item.State = pgdb.UTC(&until), "waiting_cooldown"
		}
		if ch.AuthFailed {
			item.State = "authentication_required"
		}
		if !settings.Enabled || !ch.Enabled {
			item.State = "disabled"
		}
		if runtime := s.runtime[ch.ID]; runtime != nil && runtime.batchSize > 0 {
			item.BatchSize, item.State = runtime.batchSize, "running"
			if runtime.config.Revision != ch.Revision {
				item.State = "updating"
			}
			if !settings.Enabled || !ch.Enabled {
				item.State = "stopping"
			}
		}
		if ch.Deleting {
			item.State = "removing"
		}
		status.Channels = append(status.Channels, item)
	}
	return status, nil
}
