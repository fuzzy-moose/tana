package metadataproxy

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

func (s *Service) Import(ctx context.Context, input collectorapi.MetadataProxyImportInput) (collectorapi.MetadataProxyImportResult, error) {
	entries, err := parseProxyList(input.Proxies, input.Protocol)
	if err != nil {
		return collectorapi.MetadataProxyImportResult{}, err
	}
	result, err := s.importChannels(ctx, entries, input.Enabled)
	if err != nil {
		return collectorapi.MetadataProxyImportResult{}, err
	}
	result.Status, err = s.Status(ctx)
	return result, err
}

func (s *Service) importChannels(ctx context.Context, entries []collectorapi.MetadataProxyInput, enabled bool) (collectorapi.MetadataProxyImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := collectorapi.MetadataProxyImportResult{}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	for _, entry := range entries {
		password := ""
		if entry.Password != nil {
			password = *entry.Password
		}
		// Duplicates retain their settings, credentials, retry state and bans,
		// including channels still finishing removal. Recreated endpoints also
		// inherit bans observed before they could be persisted.
		if until := s.observed["endpoint:"+entry.ProxyURL]; !until.IsZero() {
			if err := saveBan(ctx, tx, entry.ProxyURL, until); err != nil {
				return result, err
			}
		}
		saved, err := tx.Exec(ctx, `INSERT INTO metadata_proxy_channels
			(id, name, endpoint, username, password, user_agent, enabled, ban_until, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7,
			GREATEST($8::timestamptz, (SELECT until_at FROM metadata_proxy_bans WHERE endpoint = $3)), $9)
			ON CONFLICT (endpoint) DO NOTHING`, rand.Text(), entry.Name, entry.ProxyURL, entry.Username, password,
			entry.UserAgent, enabled, pgdb.NullTime(s.observed["endpoint:"+entry.ProxyURL]), time.Now())
		if err != nil {
			return result, err
		}
		n := saved.RowsAffected()
		result.Added += int(n)
		result.Duplicates += 1 - int(n)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	s.notify()
	return result, nil
}
