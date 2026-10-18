package metadataproxy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/pgdb"
)

type banState struct {
	service      *Service
	id, endpoint string
}

func (b *banState) Until(ctx context.Context) (time.Time, error) {
	b.service.mu.Lock()
	defer b.service.mu.Unlock()
	return b.service.banUntil(ctx, b.id, b.endpoint)
}

func (s *Service) banUntil(ctx context.Context, id, endpoint string) (time.Time, error) {
	var until *time.Time
	err := s.db.QueryRow(ctx, `SELECT GREATEST(
		(SELECT ban_until FROM metadata_proxy_channels WHERE id = $1),
		(SELECT until_at FROM metadata_proxy_bans WHERE endpoint = $2))`, id, endpoint).Scan(&until)
	channelBan, endpointBan := s.observed["channel:"+id], s.observed["endpoint:"+endpoint]
	return pgdb.Latest(until, &channelBan, &endpointBan), err
}

// observe moves an in-memory ban deadline forward only.
func (s *Service) observe(key string, until time.Time) {
	if until.After(s.observed[key]) {
		s.observed[key] = until
	}
}

// ResetBans lifts every channel and endpoint ban and the retry waits they caused.
func (s *Service) ResetBans(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, statement := range []string{
		`UPDATE metadata_proxy_bans SET until_at = NULL`,
		`UPDATE metadata_proxy_channels SET ban_until = NULL`,
		`UPDATE metadata_proxy_channels SET failures = 0, retry_at = NULL, last_error = '' WHERE last_error = 'panda_banned'`,
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	clear(s.observed)
	s.notify()
	return nil
}

func (b *banState) Extend(ctx context.Context, until time.Time) error {
	s := b.service
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the storage precision so remembered and persisted deadlines agree.
	until = until.Truncate(time.Microsecond)
	// Preserve observed bans in memory even if storage fails, matching the
	// main fetcher's ban behavior. Client edits must not reset this guard.
	s.observe("channel:"+b.id, until)
	s.observe("endpoint:"+b.endpoint, until)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := saveBan(ctx, tx, b.endpoint, until); err != nil {
		return err
	}
	var endpoint string
	err = tx.QueryRow(ctx, `SELECT endpoint FROM metadata_proxy_channels WHERE id = $1`, b.id).Scan(&endpoint)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if endpoint != "" {
		s.observe("endpoint:"+endpoint, until)
		if err := saveBan(ctx, tx, endpoint, until); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE metadata_proxy_channels SET ban_until = GREATEST(ban_until, $1) WHERE id = $2`, until, b.id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
