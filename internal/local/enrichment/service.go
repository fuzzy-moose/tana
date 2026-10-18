// Package enrichment durably applies collector metadata after local imports.
package enrichment

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/enrichment/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

const pollInterval = time.Minute

type lookupClient interface {
	Lookup(context.Context, []int64) (collectorapi.LookupResult, error)
}

type Service struct {
	db      *pgxpool.Pool
	q       *dbgen.Queries
	client  lookupClient
	logger  *slog.Logger
	cancel  context.CancelFunc
	workers sync.WaitGroup
}

// New starts a worker for both new imports and work retained across restarts.
// The caller owns db and must close this service before closing storage.
func New(ctx context.Context, db *pgxpool.Pool, client lookupClient, logger *slog.Logger) *Service {
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, q: dbgen.New(db), client: client, logger: logger, cancel: cancel}
	s.workers.Go(func() { s.run(ctx) })
	return s
}

func (s *Service) Close() {
	s.cancel()
	s.workers.Wait()
}

// EnqueueTx registers an inferred ID atomically with its newly imported gallery.
// Sources without the trailing ID convention need no enrichment work.
func (s *Service) EnqueueTx(ctx context.Context, tx pgx.Tx, galleryID int64, name string, kind source.Kind) error {
	id := candidateID(name, kind)
	if id == 0 {
		return nil
	}
	return s.q.WithTx(tx).Enqueue(ctx, dbgen.EnqueueParams{GalleryID: galleryID, PandaID: id})
}

func (s *Service) run(ctx context.Context) {
	for ctx.Err() == nil {
		delay, err := s.step(ctx, time.Now())
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("panda_enrichment_failed", "error", err)
			delay = pollInterval
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Service) step(ctx context.Context, at time.Time) (time.Duration, error) {
	rows, err := s.q.Due(ctx, dbgen.DueParams{Now: at, RowLimit: collectorapi.MaxLookupSize})
	if err != nil || len(rows) == 0 {
		return time.Second, err
	}
	ids := make([]int64, 0, len(rows))
	seen := map[int64]bool{}
	for _, row := range rows {
		if !seen[row.PandaID] {
			ids = append(ids, row.PandaID)
			seen[row.PandaID] = true
		}
	}
	result, lookupErr := s.client.Lookup(ctx, ids)
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	available := map[int64]collectorapi.CollectedMetadata{}
	for _, value := range result.Galleries {
		available[value.Metadata.ID] = value
	}
	pending := map[int64]bool{}
	for _, id := range result.PendingIDs {
		pending[id] = true
	}
	failed := map[int64]bool{}
	for _, id := range result.FailedIDs {
		failed[id] = true
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	galleryIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		galleryIDs = append(galleryIDs, row.GalleryID)
	}
	// Deleting a gallery during lookup also deletes its pending enrichment;
	// work another worker holds or already rescheduled is left to it.
	claimed, err := q.ClaimDue(ctx, dbgen.ClaimDueParams{GalleryIds: galleryIDs, Now: at})
	if err != nil {
		return 0, err
	}
	galleries := gallery.NewStore(s.db)
	for _, row := range claimed {
		if lookupErr != nil || pending[row.PandaID] {
			failures := int64(0)
			delay := pollInterval
			if lookupErr != nil {
				failures = min(row.Failures+1, 7)
				delay = min(pollInterval*time.Duration(1<<(failures-1)), time.Hour)
			}
			if err := q.Retry(ctx, dbgen.RetryParams{GalleryID: row.GalleryID, NextAttemptAt: new(at.Add(delay)), Failures: failures}); err != nil {
				return 0, err
			}
			continue
		}
		if value, ok := available[row.PandaID]; ok {
			values, diagnostics := metadataValues(value.Metadata)
			for _, diagnostic := range diagnostics {
				s.logger.Warn("panda_enrichment_metadata_invalid", "gallery_id", row.GalleryID, "panda_id", row.PandaID, "error", diagnostic)
			}
			if err := galleries.ApplyMetadataTx(ctx, tx, row.GalleryID, values); err != nil {
				return 0, err
			}
		} else {
			status := "unknown"
			if failed[row.PandaID] {
				status = "failed"
			}
			s.logger.Info("panda_enrichment_unresolved", "gallery_id", row.GalleryID, "panda_id", row.PandaID, "status", status)
		}
		if err := q.Complete(ctx, row.GalleryID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	if lookupErr != nil {
		s.logger.Warn("collector_lookup_failed", "error", lookupErr)
	}
	if len(claimed) == 0 {
		// Wait rather than repeat the lookup while another transaction holds the work.
		return time.Second, nil
	}
	return 0, nil
}
