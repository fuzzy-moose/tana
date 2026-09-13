package metadataproxy

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	insert, err := tx.PrepareContext(ctx, `INSERT INTO metadata_proxy_channels
		(id, name, endpoint, username, password, user_agent, enabled, ban_until, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, max(?, coalesce((SELECT until_at FROM metadata_proxy_bans WHERE endpoint = ?), 0)), ?)
		ON CONFLICT (endpoint) DO NOTHING`)
	if err != nil {
		return result, err
	}
	defer insert.Close()
	for _, entry := range entries {
		password := ""
		if entry.Password != nil {
			password = *entry.Password
		}
		// Duplicates retain their settings, credentials, retry state and bans,
		// including channels still finishing removal. Recreated endpoints also
		// inherit bans observed before they could be persisted.
		if until := s.observed["endpoint:"+entry.ProxyURL]; until > 0 {
			if err := saveBan(ctx, tx, entry.ProxyURL, until); err != nil {
				return result, err
			}
		}
		saved, err := insert.ExecContext(ctx, rand.Text(), entry.Name, entry.ProxyURL, entry.Username, password,
			entry.UserAgent, enabled, s.observed["endpoint:"+entry.ProxyURL], entry.ProxyURL, time.Now().UnixMilli())
		if err != nil {
			return result, err
		}
		n, err := saved.RowsAffected()
		if err != nil {
			return result, err
		}
		result.Added += int(n)
		result.Duplicates += 1 - int(n)
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	s.notify()
	return result, nil
}
