package metadataproxy

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type banState struct {
	service      *Service
	id, endpoint string
}

func (b *banState) Until(ctx context.Context) (time.Time, error) {
	b.service.mu.Lock()
	defer b.service.mu.Unlock()
	until, err := b.service.banUntil(ctx, b.id, b.endpoint)
	return time.UnixMilli(until), err
}

func (s *Service) banUntil(ctx context.Context, id, endpoint string) (int64, error) {
	var until int64
	err := s.db.QueryRowContext(ctx, `SELECT max(
		coalesce((SELECT ban_until FROM metadata_proxy_channels WHERE id = ?), 0),
		coalesce((SELECT until_at FROM metadata_proxy_bans WHERE endpoint = ?), 0))`, id, endpoint).Scan(&until)
	return max(until, s.observed["channel:"+id], s.observed["endpoint:"+endpoint]), err
}

func (b *banState) Extend(ctx context.Context, until time.Time) error {
	s := b.service
	s.mu.Lock()
	defer s.mu.Unlock()
	at := until.UnixMilli()
	// Preserve observed bans in memory even if storage fails, matching the
	// main fetcher's ban behavior. Client edits must not reset this guard.
	s.observed["channel:"+b.id] = max(s.observed["channel:"+b.id], at)
	s.observed["endpoint:"+b.endpoint] = max(s.observed["endpoint:"+b.endpoint], at)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := saveBan(ctx, tx, b.endpoint, at); err != nil {
		return err
	}
	var endpoint string
	err = tx.QueryRowContext(ctx, `SELECT endpoint FROM metadata_proxy_channels WHERE id = ?`, b.id).Scan(&endpoint)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if endpoint != "" {
		s.observed["endpoint:"+endpoint] = max(s.observed["endpoint:"+endpoint], at)
		if err := saveBan(ctx, tx, endpoint, at); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE metadata_proxy_channels SET ban_until = max(ban_until, ?) WHERE id = ?`, at, b.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
