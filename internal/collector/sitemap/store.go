package sitemap

import (
	"context"
	"io"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func (s *Service) transaction(ctx context.Context, work func(*dbgen.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := work(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func updateRun(ctx context.Context, q *dbgen.Queries, run dbgen.SitemapRun) error {
	return q.UpdateRun(ctx, dbgen.UpdateRunParams{State: run.State, Force: run.Force, IndexUrl: run.IndexUrl,
		IndexReady: run.IndexReady, IndexFailures: run.IndexFailures, StartedAt: run.StartedAt,
		FinishedAt: run.FinishedAt, RetryAt: run.RetryAt, NextRequestAt: run.NextRequestAt, LastError: run.LastError})
}

func updateChild(ctx context.Context, q *dbgen.Queries, child dbgen.SitemapChild) error {
	return q.UpdateChild(ctx, dbgen.UpdateChildParams{State: child.State, Failures: child.Failures,
		RetryAt: child.RetryAt, LastError: child.LastError, ID: child.ID})
}

type storageError struct{ err error }

func (e *storageError) Error() string { return e.err.Error() }
func (e *storageError) Unwrap() error { return e.err }

func (s *Service) importChild(ctx context.Context, childID int64, reader io.Reader) error {
	refs := make([]panda.GalleryRef, 0, batchSize)
	var invalid int64
	flush := func() error {
		if len(refs) == 0 && invalid == 0 {
			return nil
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		err := s.transaction(ctx, func(q *dbgen.Queries) error {
			var imported int64
			for _, ref := range refs {
				n, err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: ref.ID, Token: ref.Token})
				if err != nil {
					return err
				}
				imported += n
			}
			return q.AddChildCounts(ctx, dbgen.AddChildCountsParams{ID: childID, Found: int64(len(refs)), Imported: imported, Invalid: invalid})
		})
		refs, invalid = refs[:0], 0
		if err != nil {
			return &storageError{err}
		}
		return nil
	}
	invalidLocations, err := panda.ParseSitemap(reader, func(ref panda.GalleryRef) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		refs = append(refs, ref)
		if len(refs) >= batchSize {
			return flush()
		}
		return nil
	})
	invalid = invalidLocations
	// Broken XML preserves all complete entries already parsed, including the
	// final short batch. Validators are committed only by successful completion.
	if flushErr := flush(); flushErr != nil {
		return flushErr
	}
	return err
}
