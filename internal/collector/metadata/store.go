package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db *sql.DB
	q  *dbgen.Queries
}

// complete commits successful metadata and related references together, so a
// restart cannot lose discovered work after marking its source collected.
func (s *store) complete(ctx context.Context, refs []panda.GalleryRef, entries []panda.Metadata, at time.Time, main bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	requested := make(map[int64]string, len(refs))
	known := make(map[int64]string, len(refs))
	for _, ref := range refs {
		requested[ref.ID] = ref.Token
	}
	// Admit confirmed requested references before related discovery, which may
	// mention another gallery in this same batch with a conflicting token.
	for i, entry := range entries {
		token, err := q.GetGalleryToken(ctx, entry.ID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil {
			known[entry.ID] = token
		}
		if err == nil && token != requested[entry.ID] {
			entries[i] = panda.Metadata{ID: entry.ID, Error: "token_conflict"}
			continue
		}
		if entry.Error == "" {
			if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: entry.ID, Token: entry.Token}); err != nil {
				return err
			}
		}
	}
	for _, entry := range entries {
		if entry.Error == "" {
			body, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := q.SaveMetadata(ctx, dbgen.SaveMetadataParams{GalleryID: entry.ID, Body: body, RefreshedAt: at.UnixMilli()}); err != nil {
				return err
			}
			if err := catalog.Project(ctx, tx, entry); err != nil {
				return err
			}
			for _, ref := range []panda.GalleryRef{
				{ID: entry.ParentID, Token: entry.ParentToken},
				{ID: entry.CurrentID, Token: entry.CurrentToken},
				{ID: entry.FirstID, Token: entry.FirstToken},
			} {
				if ref.ID > 0 && ref.Token != "" {
					if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: ref.ID, Token: ref.Token}); err != nil {
						return err
					}
				}
			}
		}
		if err := q.RecordAttempt(ctx, dbgen.RecordAttemptParams{
			GalleryID: entry.ID, Token: requested[entry.ID], MetadataAttemptedAt: nullableMillis(at),
			MetadataError: sql.NullString{String: entry.Error, Valid: entry.Error != ""},
		}); err != nil {
			return err
		}
		result := dbgen.CompleteFetchParams{GalleryID: entry.ID, Token: requested[entry.ID], Status: "failed", Error: entry.Error}
		if entry.Error == "" {
			result.Status, result.RefreshedAt = "successful", nullableMillis(at)
		}
		if err := q.CompleteFetch(ctx, result); err != nil {
			return err
		}
	}
	// Related discovery in any response can make a failed reference known.
	// Settle imports after all discovery, independently of response order.
	for _, entry := range entries {
		if _, exists := known[entry.ID]; !exists && entry.Error != "" {
			token, err := q.GetGalleryToken(ctx, entry.ID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil {
				known[entry.ID] = token
			}
		}
		if err := completeImportedReference(ctx, tx, panda.GalleryRef{ID: entry.ID, Token: requested[entry.ID]}, entry.Error == "", known[entry.ID] == requested[entry.ID]); err != nil {
			return err
		}
		if token, ok := known[entry.ID]; ok {
			if err := settleImportedGallery(ctx, tx, entry.ID, token); err != nil {
				return err
			}
		} else if entry.Error == "" {
			if err := settleImportedGallery(ctx, tx, entry.ID, entry.Token); err != nil {
				return err
			}
		}
	}
	if err := completeReferenceImports(ctx, tx, at); err != nil {
		return err
	}
	if err := q.CompleteFetchJobs(ctx, nullableMillis(at)); err != nil {
		return err
	}
	if main {
		if err := q.RecordBatchSuccess(ctx, sql.NullInt64{Int64: at.UnixMilli(), Valid: true}); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *store) maintainJobs(ctx context.Context, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	if err := q.FailConflictingFetches(ctx); err != nil {
		return err
	}
	if err := q.CompleteFetchJobs(ctx, nullableMillis(at)); err != nil {
		return err
	}
	if err := q.DeleteExpiredFetchJobs(ctx, nullableMillis(at.Add(-JobRetention))); err != nil {
		return err
	}
	if err := q.DeleteOrphanFetches(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) pendingRefs(ctx context.Context) ([]panda.GalleryRef, error) {
	return s.pendingRefsFor(ctx, false, nil)
}

func (s *store) pendingRefsFor(ctx context.Context, background bool, reserved map[int64]bool) ([]panda.GalleryRef, error) {
	// Each query selects distinct gallery IDs. This bounded over-read leaves a
	// full batch available even when every reserved gallery precedes other work.
	limit := int64(panda.MaxBatchSize + len(reserved))
	refs := make([]panda.GalleryRef, 0, panda.MaxBatchSize)
	if !background {
		queued, err := s.q.PendingFetches(ctx, limit)
		if err != nil {
			return nil, err
		}
		for _, row := range queued {
			if !reserved[row.GalleryID] && len(refs) < panda.MaxBatchSize {
				refs = append(refs, panda.GalleryRef{ID: row.GalleryID, Token: row.Token})
			}
		}
		if len(queued) != 0 {
			return refs, nil
		}
	}
	rows, err := s.q.PendingRefs(ctx, limit)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !reserved[row.GalleryID] && len(refs) < panda.MaxBatchSize {
			refs = append(refs, panda.GalleryRef{ID: row.GalleryID, Token: row.Token})
		}
	}
	if len(rows) != 0 {
		return refs, nil
	}
	return s.pendingImportRefs(ctx, reserved)
}
