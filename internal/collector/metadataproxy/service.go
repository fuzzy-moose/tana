package metadataproxy

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/panda"
	"golang.org/x/time/rate"
)

type Service struct {
	db       *sql.DB
	metadata *metadata.Service
	config   panda.Config
	logger   *slog.Logger
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	wake     chan struct{}
	mu       sync.Mutex
	runtime  map[string]*channelRuntime
	observed map[string]int64
	verifyIP func(context.Context, http.RoundTripper) error
}

type channelRuntime struct {
	config    channel
	limiter   *rate.Limiter
	client    *panda.Client
	transport *http.Transport
	batchSize int
	holdUntil time.Time
}

func New(ctx context.Context, db *sql.DB, metadataService *metadata.Service, config panda.Config, logger *slog.Logger) (*Service, error) {
	return newService(ctx, db, metadataService, config, logger, newIPVerifier().verify)
}

func newService(ctx context.Context, db *sql.DB, metadataService *metadata.Service, config panda.Config, logger *slog.Logger,
	verifyIP func(context.Context, http.RoundTripper) error) (*Service, error) {
	if _, err := panda.NewRateLimiter(config.RateInterval, 1); err != nil {
		return nil, err
	}
	if _, err := panda.NewClient(config.APIURL, nil); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, metadata: metadataService, config: config, logger: logger, cancel: cancel,
		wake: make(chan struct{}, 1), runtime: make(map[string]*channelRuntime), observed: make(map[string]int64), verifyIP: verifyIP}
	if _, _, err := s.channels(ctx); err != nil {
		cancel()
		return nil, err
	}
	s.workers.Go(func() { s.run(ctx) })
	return s, nil
}

func (s *Service) Close() {
	s.cancel()
	s.workers.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, runtime := range s.runtime {
		if runtime.transport != nil {
			runtime.transport.CloseIdleConnections()
		}
	}
}

func (s *Service) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		delay := time.Second
		if err := s.dispatch(ctx); err != nil && ctx.Err() == nil {
			s.logger.Error("metadata_proxy_dispatch_failed", "error", err)
			delay = time.Minute
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *Service) dispatch(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM metadata_proxy_channels ORDER BY rowid`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.dispatchChannel(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Release the configuration lock between channels so status, bans and results
// can progress during a large dispatch. Read current settings under the lock:
// only an already assigned batch may finish with settings that were edited.
func (s *Service) dispatchChannel(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime := s.runtime[id]
	if runtime != nil && runtime.batchSize != 0 {
		return nil
	}
	settings, err := s.settings(ctx)
	if err != nil {
		return err
	}
	ch, err := scanChannel(s.db.QueryRowContext(ctx, `SELECT `+channelColumns+` FROM metadata_proxy_channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	lastSuccess := ch.LastSuccessAt
	if lastSuccess == 0 {
		lastSuccess = ch.CreatedAt
	}
	if ch.Deleting || (settings.AutoRemoveInactive && lastSuccess <= time.Now().Add(-5*time.Minute).UnixMilli()) {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM metadata_proxy_channels WHERE id = ?`, ch.ID); err != nil {
			return err
		}
		if runtime != nil && runtime.transport != nil {
			runtime.transport.CloseIdleConnections()
		}
		delete(s.runtime, ch.ID)
		delete(s.observed, "channel:"+ch.ID)
		return nil
	}
	if runtime != nil && runtime.holdUntil.After(time.Now()) {
		return nil
	}
	if !settings.Enabled || !ch.Enabled || ch.AuthFailed || ch.RetryAt > time.Now().UnixMilli() {
		return nil
	}
	until, err := s.banUntil(ctx, ch.ID, ch.Endpoint)
	if err != nil {
		return err
	}
	if until > time.Now().UnixMilli() {
		return nil
	}
	if runtime == nil {
		limiter, err := panda.NewRateLimiter(s.config.RateInterval, 1)
		if err != nil {
			return err
		}
		runtime = &channelRuntime{limiter: limiter}
		s.runtime[ch.ID] = runtime
	}
	if runtime.client == nil || runtime.config.Revision != ch.Revision {
		if runtime.transport != nil {
			runtime.transport.CloseIdleConnections()
		}
		client, transport, err := s.client(ch, runtime.limiter)
		if err != nil {
			return err
		}
		runtime.client, runtime.transport, runtime.config = client, transport, ch
	}
	batch, err := s.metadata.ClaimBackground(ctx)
	if err != nil {
		return err
	}
	if batch == nil {
		return nil
	}
	runtime.config = ch
	runtime.batchSize = batch.Size()
	s.workers.Go(func() { s.fetch(ctx, runtime, batch) })
	return nil
}

func (s *Service) fetch(ctx context.Context, runtime *channelRuntime, batch *metadata.Batch) {
	err := batch.Fetch(ctx, runtime.client)
	s.mu.Lock()
	defer func() {
		batch.Close()
		runtime.batchSize = 0
		s.mu.Unlock()
		s.notify()
	}()
	if ctx.Err() != nil {
		return
	}
	ch := runtime.config
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE metadata_proxy_channels SET failures = 0, retry_at = 0,
			last_error = '', last_success_at = ? WHERE id = ?`, time.Now().UnixMilli(), ch.ID)
	} else if errors.Is(err, errProxyIPLeak) || errors.Is(err, errProxyIPCheck) {
		code, _ := failure(err)
		s.logger.Warn("metadata_proxy_verification_failed", "channel_id", ch.ID, "error", code)
		// A check using old settings must not remove an edited channel.
		_, err = s.db.ExecContext(ctx, `UPDATE metadata_proxy_channels SET deleting = 1, enabled = 0,
			revision = revision + 1 WHERE id = ? AND revision = ?`, ch.ID, ch.Revision)
	} else {
		code, auth := failure(err)
		s.logger.Warn("metadata_proxy_batch_failed", "channel_id", ch.ID, "error", code)
		delay := panda.RetryDelay(ch.Failures, err, time.Now())
		retryAt := time.Now().Add(delay).UnixMilli()
		if auth {
			retryAt = 0
		}
		// An authentication failure of the old route must not block credentials
		// corrected while that request was in flight.
		_, err = s.db.ExecContext(ctx, `UPDATE metadata_proxy_channels SET failures = min(failures + 1, 7),
			retry_at = ?, last_error = ?, auth_failed = CASE WHEN endpoint = ? AND username = ? AND password = ? THEN ? ELSE auth_failed END
			WHERE id = ?`, retryAt, code, ch.Endpoint, ch.Username, ch.Password, auth, ch.ID)
	}
	if err != nil {
		runtime.holdUntil = time.Now().Add(time.Minute)
		s.logger.Error("metadata_proxy_result_record_failed", "channel_id", ch.ID, "error", err)
	}
}
