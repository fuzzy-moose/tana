// Package feed captures Panda feeds independently of parsing and reference storage.
package feed

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type Service struct {
	store       *store
	config      Config
	client      *http.Client
	logger      *slog.Logger
	trigger     chan struct{}
	refresh     chan struct{}
	mu          sync.Mutex
	active      bool
	lastError   string
	lastErrorAt *time.Time
	cancel      context.CancelFunc
	workers     sync.WaitGroup
}

// New starts the downloader and a single processor. The caller owns db.
func New(ctx context.Context, db *sql.DB, cfg Config, client *http.Client, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	s := &Service{
		store: newStore(db), config: cfg, client: client,
		logger: logger, trigger: make(chan struct{}, 1), refresh: make(chan struct{}, 1), cancel: cancel,
	}
	s.notifyProcessor()
	s.workers.Go(func() { s.processLoop(ctx) })
	s.workers.Go(func() { s.downloadLoop(ctx) })
	return s
}

func (s *Service) Close() {
	s.cancel()
	s.workers.Wait()
}

func (s *Service) notifyProcessor() {
	// Coalesce triggers without losing work arriving during the current pass.
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

func (s *Service) downloadLoop(ctx context.Context) {
	retry := false
	for ctx.Err() == nil {
		delay, err := s.store.fetchDelay(ctx, time.Now(), s.config.Interval)
		if err != nil {
			s.logger.Error("feed_schedule_failed", "error", err)
		}
		if retry || err != nil {
			delay = s.config.RetryDelay
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.refresh:
			timer.Stop()
		case <-timer.C:
		}
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		// A scheduled capture also satisfies a manual request racing its timer.
		select {
		case <-s.refresh:
		default:
		}
		s.active = true
		s.mu.Unlock()
		err = s.download(ctx)
		s.mu.Lock()
		s.active = false
		if err != nil {
			now := time.Now().UTC()
			s.lastError, s.lastErrorAt = err.Error(), &now
		} else {
			s.lastError, s.lastErrorAt = "", nil
		}
		s.mu.Unlock()
		retry = err != nil
		if err != nil && ctx.Err() == nil {
			s.logger.Error("feed_download_failed", "error", err)
		}
	}
}

func (s *Service) download(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/atom+xml, application/xml, text/xml")
	req.Header.Set("User-Agent", "Tana")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch feed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch feed: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read feed: %w", err)
	}
	id, err := s.store.capture(ctx, s.config.URL, body, time.Now())
	if err != nil {
		return fmt.Errorf("store feed: %w", err)
	}
	s.notifyProcessor()
	s.logger.Info("feed_captured", "capture_id", id, "bytes", len(body))
	return nil
}

func (s *Service) processLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.trigger:
			s.processPending(ctx)
		}
	}
}

func (s *Service) processPending(ctx context.Context) {
	// Snapshot IDs, not response bodies: failures are attempted once per pass,
	// and the backlog does not need to fit in memory as raw XML.
	ids, err := s.store.q.PendingCaptures(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("feed_pending_failed", "error", err)
		}
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		if err := s.process(ctx, id); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("feed_processing_failed", "capture_id", id, "error", err)
			if recordErr := s.store.failed(ctx, id, err); recordErr != nil {
				s.logger.Error("feed_failure_record_failed", "capture_id", id, "error", recordErr)
			}
		}
	}
}

func (s *Service) process(ctx context.Context, id int64) error {
	body, err := s.store.q.CaptureBody(ctx, id)
	if err != nil {
		return err
	}
	entries, err := panda.ParseFeed(bytes.NewReader(body))
	if err != nil {
		return err
	}
	gaps, err := s.store.complete(ctx, id, entries, time.Now())
	if err != nil {
		return err
	}
	for _, gap := range gaps {
		s.logger.Warn("feed_possible_gap", "previous_capture_id", gap.PreviousCaptureID,
			"current_capture_id", gap.CurrentCaptureID, "checked_at", time.UnixMilli(gap.CheckedAt).UTC())
	}
	s.logger.Info("feed_processed", "capture_id", id, "entries", len(entries))
	return nil
}
