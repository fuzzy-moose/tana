package metadataproxy

import (
	"context"
	"crypto/rand"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

type channel struct {
	ID, Name, Endpoint, Username, Password, UserAgent string
	Enabled, Deleting, AuthFailed                     bool
	Revision, Failures                                int64
	BanUntil, RetryAt, LastSuccessAt                  *time.Time
	CreatedAt                                         time.Time
	LastError                                         string
}

func (s *Service) settings(ctx context.Context) (collectorapi.MetadataProxySettings, error) {
	var settings collectorapi.MetadataProxySettings
	err := s.db.QueryRow(ctx, `SELECT enabled, auto_remove_inactive FROM metadata_proxy_settings WHERE id = 1`).
		Scan(&settings.Enabled, &settings.AutoRemoveInactive)
	return settings, err
}

const channelColumns = `id, name, endpoint, username, password, user_agent, enabled,
	revision, deleting, ban_until, failures, retry_at, auth_failed, last_error, last_success_at, created_at`

func scanChannel(row interface{ Scan(...any) error }) (channel, error) {
	var ch channel
	err := row.Scan(&ch.ID, &ch.Name, &ch.Endpoint, &ch.Username, &ch.Password, &ch.UserAgent, &ch.Enabled,
		&ch.Revision, &ch.Deleting, &ch.BanUntil, &ch.Failures, &ch.RetryAt, &ch.AuthFailed, &ch.LastError, &ch.LastSuccessAt, &ch.CreatedAt)
	return ch, err
}

func (s *Service) channels(ctx context.Context) (collectorapi.MetadataProxySettings, []channel, error) {
	settings, err := s.settings(ctx)
	if err != nil {
		return settings, nil, err
	}
	rows, err := s.db.Query(ctx, `SELECT `+channelColumns+` FROM metadata_proxy_channels ORDER BY sequence`)
	if err != nil {
		return settings, nil, err
	}
	defer rows.Close()
	var channels []channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return settings, nil, err
		}
		channels = append(channels, ch)
	}
	return settings, channels, rows.Err()
}

func (s *Service) SetSettings(ctx context.Context, settings collectorapi.MetadataProxySettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(ctx, `UPDATE metadata_proxy_settings SET enabled = $1, auto_remove_inactive = $2 WHERE id = 1`,
		settings.Enabled, settings.AutoRemoveInactive)
	if err == nil {
		s.notify()
	}
	return err
}

func (s *Service) Save(ctx context.Context, id string, input collectorapi.MetadataProxyInput) error {
	input, err := normalize(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old channel
	if id != "" {
		err := tx.QueryRow(ctx, `SELECT endpoint, username, password, ban_until, deleting FROM metadata_proxy_channels WHERE id = $1 FOR UPDATE`, id).
			Scan(&old.Endpoint, &old.Username, &old.Password, &old.BanUntil, &old.Deleting)
		if errors.Is(err, pgx.ErrNoRows) {
			return collectorapi.ErrProxyNotFound
		}
		if err != nil {
			return err
		}
		if old.Deleting {
			return collectorapi.ErrProxyChanging
		}
	}
	var duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM metadata_proxy_channels WHERE endpoint = $1 AND id != $2)`, input.ProxyURL, id).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return collectorapi.ErrDuplicateProxy
	}
	// Write-only credentials must not follow an edit to a different proxy.
	var password string
	if old.Endpoint == input.ProxyURL {
		password = old.Password
	}
	if input.Password != nil {
		password = *input.Password
	}
	var recorded *time.Time
	if err := tx.QueryRow(ctx, `SELECT max(until_at) FROM metadata_proxy_bans WHERE endpoint IN ($1, $2)`, input.ProxyURL, old.Endpoint).Scan(&recorded); err != nil {
		return err
	}
	channelBan, newBan, oldBan := s.observed["channel:"+id], s.observed["endpoint:"+input.ProxyURL], s.observed["endpoint:"+old.Endpoint]
	until := pgdb.Latest(recorded, old.BanUntil, &channelBan, &newBan, &oldBan)
	if err := saveBan(ctx, tx, input.ProxyURL, until); err != nil {
		return err
	}
	if old.Endpoint != "" {
		if err := saveBan(ctx, tx, old.Endpoint, until); err != nil {
			return err
		}
	}
	if id == "" {
		id = rand.Text()
		_, err = tx.Exec(ctx, `INSERT INTO metadata_proxy_channels
			(id, name, endpoint, username, password, user_agent, enabled, ban_until, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			id, input.Name, input.ProxyURL, input.Username, password, input.UserAgent, input.Enabled, pgdb.NullTime(until), time.Now())
	} else {
		changedAuth := old.Endpoint != input.ProxyURL || old.Username != input.Username || old.Password != password
		_, err = tx.Exec(ctx, `UPDATE metadata_proxy_channels SET name = $1, endpoint = $2, username = $3, password = $4,
			user_agent = $5, enabled = $6, ban_until = GREATEST(ban_until, $7), revision = revision + 1,
			auth_failed = CASE WHEN $8::boolean THEN false ELSE auth_failed END WHERE id = $9`,
			input.Name, input.ProxyURL, input.Username, password, input.UserAgent, input.Enabled, pgdb.NullTime(until), changedAuth, id)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Physical deletion is deferred until any assigned batch has saved its
	// result and observed ban, including one arriving after this request.
	result, err := s.db.Exec(ctx, `UPDATE metadata_proxy_channels SET deleting = true, enabled = false, revision = revision + 1 WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return collectorapi.ErrProxyNotFound
	}
	s.notify()
	return nil
}

func saveBan(ctx context.Context, tx pgx.Tx, endpoint string, until time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO metadata_proxy_bans (endpoint, until_at) VALUES ($1, $2)
		ON CONFLICT (endpoint) DO UPDATE SET until_at = GREATEST(metadata_proxy_bans.until_at, excluded.until_at)`, endpoint, pgdb.NullTime(until))
	return err
}
