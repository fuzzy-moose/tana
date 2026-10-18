package metadataproxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type Service struct {
	db       *pgxpool.Pool
	metadata *metadata.Service
	config   panda.Config
	logger   *slog.Logger
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	wake     chan struct{}
	mu       sync.Mutex
	runtime  map[string]*channelRuntime
	observed map[string]time.Time
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

func New(ctx context.Context, db *pgxpool.Pool, metadataService *metadata.Service, config panda.Config, logger *slog.Logger) (*Service, error) {
	return newService(ctx, db, metadataService, config, logger, newIPVerifier().verify)
}

func newService(ctx context.Context, db *pgxpool.Pool, metadataService *metadata.Service, config panda.Config, logger *slog.Logger,
	verifyIP func(context.Context, http.RoundTripper) error) (*Service, error) {
	if _, err := panda.NewRateLimiter(config.RateInterval, 1); err != nil {
		return nil, err
	}
	if _, err := panda.NewClient(config.APIURL, nil); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, metadata: metadataService, config: config, logger: logger, cancel: cancel,
		wake: make(chan struct{}, 1), runtime: make(map[string]*channelRuntime), observed: make(map[string]time.Time), verifyIP: verifyIP}
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
	rows, err := s.db.Query(ctx, `SELECT id FROM metadata_proxy_channels ORDER BY sequence`)
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
	rows.Close()
	if err := rows.Err(); err != nil {
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
	ch, err := scanChannel(s.db.QueryRow(ctx, `SELECT `+channelColumns+` FROM metadata_proxy_channels WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	lastSuccess := ch.CreatedAt
	if ch.LastSuccessAt != nil {
		lastSuccess = *ch.LastSuccessAt
	}
	if ch.Deleting || (settings.AutoRemoveInactive && !lastSuccess.After(time.Now().Add(-5*time.Minute))) {
		if _, err := s.db.Exec(ctx, `DELETE FROM metadata_proxy_channels WHERE id = $1`, ch.ID); err != nil {
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
	if !settings.Enabled || !ch.Enabled || ch.AuthFailed || (ch.RetryAt != nil && ch.RetryAt.After(time.Now())) {
		return nil
	}
	until, err := s.banUntil(ctx, ch.ID, ch.Endpoint)
	if err != nil {
		return err
	}
	if until.After(time.Now()) {
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
		_, err = s.db.Exec(ctx, `UPDATE metadata_proxy_channels SET failures = 0, retry_at = NULL,
			last_error = '', last_success_at = $1 WHERE id = $2`, time.Now(), ch.ID)
	} else if errors.Is(err, errProxyIPLeak) || errors.Is(err, errProxyIPCheck) {
		code, _ := failure(err)
		s.logger.Warn("metadata_proxy_verification_failed", "channel_id", ch.ID, "error", code)
		// A check using old settings must not remove an edited channel.
		_, err = s.db.Exec(ctx, `UPDATE metadata_proxy_channels SET deleting = true, enabled = false,
			revision = revision + 1 WHERE id = $1 AND revision = $2`, ch.ID, ch.Revision)
	} else {
		code, auth := failure(err)
		s.logger.Warn("metadata_proxy_batch_failed", "channel_id", ch.ID, "error", code)
		delay := panda.RetryDelay(ch.Failures, err, time.Now())
		var retryAt *time.Time
		if !auth {
			retryAt = new(time.Now().Add(delay))
		}
		// An authentication failure of the old route must not block credentials
		// corrected while that request was in flight.
		_, err = s.db.Exec(ctx, `UPDATE metadata_proxy_channels SET failures = LEAST(failures + 1, 7),
			retry_at = $1, last_error = $2, auth_failed = CASE WHEN endpoint = $3 AND username = $4 AND password = $5 THEN $6::boolean ELSE auth_failed END
			WHERE id = $7`, retryAt, code, ch.Endpoint, ch.Username, ch.Password, auth, ch.ID)
	}
	if err != nil {
		runtime.holdUntil = time.Now().Add(time.Minute)
		s.logger.Error("metadata_proxy_result_record_failed", "channel_id", ch.ID, "error", err)
	}
}
