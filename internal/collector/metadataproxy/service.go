package metadataproxy

import (
	"context"
	"database/sql"
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
	if _, err := panda.NewRateLimiter(config.RateInterval, 1); err != nil {
		return nil, err
	}
	if _, err := panda.NewClient(config.APIURL, nil); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, metadata: metadataService, config: config, logger: logger, cancel: cancel,
		wake: make(chan struct{}, 1), runtime: make(map[string]*channelRuntime), observed: make(map[string]int64)}
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

// Configuration and dispatch share a lock; once a change is saved, only a
// batch already assigned to that channel may finish with its old settings.
func (s *Service) dispatch(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enabled, channels, err := s.channels(ctx)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		runtime := s.runtime[ch.ID]
		if runtime != nil && runtime.batchSize != 0 {
			continue
		}
		if ch.Deleting {
			if _, err := s.db.ExecContext(ctx, `DELETE FROM metadata_proxy_channels WHERE id = ?`, ch.ID); err != nil {
				return err
			}
			if runtime != nil && runtime.transport != nil {
				runtime.transport.CloseIdleConnections()
			}
			delete(s.runtime, ch.ID)
			delete(s.observed, "channel:"+ch.ID)
			continue
		}
		if runtime != nil && runtime.holdUntil.After(time.Now()) {
			continue
		}
		if !enabled || !ch.Enabled || ch.AuthFailed || ch.RetryAt > time.Now().UnixMilli() {
			continue
		}
		until, err := s.banUntil(ctx, ch.ID, ch.Endpoint)
		if err != nil {
			return err
		}
		if until > time.Now().UnixMilli() {
			continue
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
			continue
		}
		runtime.config = ch
		runtime.batchSize = batch.Size()
		s.workers.Go(func() { s.fetch(ctx, runtime, batch) })
	}
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
