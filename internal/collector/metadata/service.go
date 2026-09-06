// Package metadata collects and retains Panda metadata independently of feeds.
package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type Service struct {
	store   *store
	client  *panda.Client
	logger  *slog.Logger
	cancel  context.CancelFunc
	workers sync.WaitGroup
	wake    chan struct{}
}

// New starts a single collector that also drains work persisted before startup.
// The caller owns db and the client's timeout and rate limiting.
func New(ctx context.Context, db *sql.DB, client *panda.Client, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{store: &store{db: db, q: dbgen.New(db)}, client: client, logger: logger, cancel: cancel, wake: make(chan struct{}, 1)}
	s.workers.Go(func() { s.run(ctx) })
	return s
}

func (s *Service) Close() {
	s.cancel()
	s.workers.Wait()
}

// Get returns retained metadata, or sql.ErrNoRows when none has been collected.
func (s *Service) Get(ctx context.Context, galleryID int64) (collectorapi.CollectedMetadata, error) {
	row, err := s.store.q.GetMetadata(ctx, galleryID)
	if err != nil {
		return collectorapi.CollectedMetadata{}, err
	}
	var value panda.Metadata
	if err := json.Unmarshal(row.Body, &value); err != nil {
		return collectorapi.CollectedMetadata{}, fmt.Errorf("decode collected metadata: %w", err)
	}
	return collectorapi.CollectedMetadata{Metadata: value, RefreshedAt: time.UnixMilli(row.RefreshedAt).UTC()}, nil
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		delay, err := s.collect(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("metadata_collection_failed", "error", err)
			var recordErr error
			delay, recordErr = s.store.failed(ctx, err, time.Now())
			if recordErr != nil {
				s.logger.Error("metadata_failure_record_failed", "error", recordErr)
				delay = time.Minute
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-s.wake:
			timer.Stop()
		}
	}
}

func (s *Service) collect(ctx context.Context) (time.Duration, error) {
	if err := s.store.maintainJobs(ctx, time.Now()); err != nil {
		return 0, err
	}
	retry, err := s.store.q.RetryState(ctx)
	if err != nil {
		return 0, err
	}
	if delay := time.Until(time.UnixMilli(retry.NextAttemptAt)); delay > 0 {
		return delay, nil
	}
	refs, err := s.store.pendingRefs(ctx)
	if err != nil || len(refs) == 0 {
		return time.Second, err
	}
	entries, err := s.client.GetMetadata(ctx, refs)
	if err != nil {
		var httpErr *panda.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode < 400 || httpErr.StatusCode >= 500 ||
			httpErr.StatusCode == http.StatusRequestTimeout || httpErr.StatusCode == http.StatusTooManyRequests {
			return 0, err
		}
		// Permanent request rejection finishes these entries instead of retrying
		// forever. Transport errors, throttling and server failures share backoff.
		entries = make([]panda.Metadata, len(refs))
		for i, ref := range refs {
			entries[i] = panda.Metadata{ID: ref.ID, Error: err.Error()}
		}
	}
	// Match by ID, not response position. Reject incomplete or unrelated batches.
	wanted := make(map[int64]string, len(refs))
	for _, ref := range refs {
		wanted[ref.ID] = ref.Token
	}
	for i, entry := range entries {
		token, ok := wanted[entry.ID]
		if !ok {
			return 0, fmt.Errorf("unexpected or duplicate Panda gallery %d", entry.ID)
		}
		if entry.Error == "" && entry.Token != token {
			entries[i] = panda.Metadata{ID: entry.ID, Error: "token_mismatch"}
		}
		delete(wanted, entry.ID)
	}
	if len(wanted) != 0 {
		return 0, fmt.Errorf("Panda response omitted %d galleries", len(wanted))
	}
	if err := s.store.complete(ctx, refs, entries, time.Now()); err != nil {
		return 0, err
	}
	collected := 0
	for _, entry := range entries {
		if entry.Error != "" {
			s.logger.Warn("metadata_gallery_failed", "gallery_id", entry.ID, "error", entry.Error)
		} else {
			collected++
		}
	}
	s.logger.Info("metadata_batch_completed", "collected", collected, "failed", len(entries)-collected)
	return 0, nil
}
