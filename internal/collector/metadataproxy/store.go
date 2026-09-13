package metadataproxy

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

type channel struct {
	ID, Name, Endpoint, Username, Password, UserAgent    string
	Enabled, Deleting, AuthFailed                        bool
	Revision, BanUntil, Failures, RetryAt, LastSuccessAt int64
	LastError                                            string
}

func (s *Service) channels(ctx context.Context) (bool, []channel, error) {
	var enabled bool
	if err := s.db.QueryRowContext(ctx, `SELECT enabled FROM metadata_proxy_settings WHERE id = 1`).Scan(&enabled); err != nil {
		return false, nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, endpoint, username, password, user_agent, enabled,
		revision, deleting, ban_until, failures, retry_at, auth_failed, last_error, last_success_at FROM metadata_proxy_channels ORDER BY rowid`)
	if err != nil {
		return false, nil, err
	}
	defer rows.Close()
	var channels []channel
	for rows.Next() {
		var ch channel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Endpoint, &ch.Username, &ch.Password, &ch.UserAgent, &ch.Enabled,
			&ch.Revision, &ch.Deleting, &ch.BanUntil, &ch.Failures, &ch.RetryAt, &ch.AuthFailed, &ch.LastError, &ch.LastSuccessAt); err != nil {
			return false, nil, err
		}
		channels = append(channels, ch)
	}
	return enabled, channels, rows.Err()
}

func (s *Service) SetEnabled(ctx context.Context, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, `UPDATE metadata_proxy_settings SET enabled = ? WHERE id = 1`, enabled)
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var old channel
	if id != "" {
		err := tx.QueryRowContext(ctx, `SELECT endpoint, username, password, ban_until, deleting FROM metadata_proxy_channels WHERE id = ?`, id).
			Scan(&old.Endpoint, &old.Username, &old.Password, &old.BanUntil, &old.Deleting)
		if errors.Is(err, sql.ErrNoRows) {
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
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM metadata_proxy_channels WHERE endpoint = ? AND id != ?)`, input.ProxyURL, id).Scan(&duplicate); err != nil {
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
	var recorded int64
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(until_at), 0) FROM metadata_proxy_bans WHERE endpoint IN (?, ?)`, input.ProxyURL, old.Endpoint).Scan(&recorded); err != nil {
		return err
	}
	until := max(recorded, old.BanUntil, s.observed["channel:"+id], s.observed["endpoint:"+input.ProxyURL], s.observed["endpoint:"+old.Endpoint])
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
		_, err = tx.ExecContext(ctx, `INSERT INTO metadata_proxy_channels
			(id, name, endpoint, username, password, user_agent, enabled, ban_until) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			id, input.Name, input.ProxyURL, input.Username, password, input.UserAgent, input.Enabled, until)
	} else {
		changedAuth := old.Endpoint != input.ProxyURL || old.Username != input.Username || old.Password != password
		_, err = tx.ExecContext(ctx, `UPDATE metadata_proxy_channels SET name = ?, endpoint = ?, username = ?, password = ?,
			user_agent = ?, enabled = ?, ban_until = ?, revision = revision + 1,
			auth_failed = CASE WHEN ? THEN 0 ELSE auth_failed END WHERE id = ?`,
			input.Name, input.ProxyURL, input.Username, password, input.UserAgent, input.Enabled, until, changedAuth, id)
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
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
	result, err := s.db.ExecContext(ctx, `UPDATE metadata_proxy_channels SET deleting = 1, enabled = 0, revision = revision + 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return collectorapi.ErrProxyNotFound
	}
	s.notify()
	return nil
}

func saveBan(ctx context.Context, tx *sql.Tx, endpoint string, until int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO metadata_proxy_bans (endpoint, until_at) VALUES (?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET until_at = max(until_at, excluded.until_at)`, endpoint, until)
	return err
}
