package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db *pgxpool.Pool
	q  *dbgen.Queries
}

// complete commits successful metadata and related references together, so a
// restart cannot lose discovered work after marking its source collected.
func (s *store) complete(ctx context.Context, refs []panda.GalleryRef, entries []panda.Metadata, at time.Time, main bool) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
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
			if err := q.SaveMetadata(ctx, dbgen.SaveMetadataParams{GalleryID: entry.ID, Body: body, RefreshedAt: at}); err != nil {
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
			GalleryID: entry.ID, Token: requested[entry.ID], MetadataAttemptedAt: &at,
			MetadataError: pgtype.Text{String: entry.Error, Valid: entry.Error != ""},
		}); err != nil {
			return err
		}
		result := dbgen.CompleteFetchParams{GalleryID: entry.ID, Token: requested[entry.ID], Status: "failed", Error: entry.Error}
		if entry.Error == "" {
			result.Status, result.RefreshedAt = "successful", &at
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
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
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
	if err := q.CompleteFetchJobs(ctx, &at); err != nil {
		return err
	}
	if main {
		if err := q.RecordBatchSuccess(ctx, &at); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *store) maintainJobs(ctx context.Context, at time.Time) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	if err := q.FailConflictingFetches(ctx); err != nil {
		return err
	}
	if err := q.CompleteFetchJobs(ctx, &at); err != nil {
		return err
	}
	if err := q.DeleteExpiredFetchJobs(ctx, new(at.Add(-JobRetention))); err != nil {
		return err
	}
	if err := q.DeleteOrphanFetches(ctx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *store) pendingRefs(ctx context.Context) ([]panda.GalleryRef, error) {
	return s.pendingRefsFor(ctx, false, nil)
}

func (s *store) pendingRefsFor(ctx context.Context, background bool, reserved map[int64]bool) ([]panda.GalleryRef, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	refs, err := s.claimRefs(ctx, tx, background, reserved)
	if err != nil {
		return nil, err
	}
	return refs, tx.Commit(ctx)
}

// claimRefs selects a batch in tx with SKIP LOCKED, passing over reserved
// galleries and rows another claim holds. The locks end with tx, before the
// upstream request; the dispatch lock's reservations cover the request, and
// admission only completes work that is still pending.
func (s *store) claimRefs(ctx context.Context, tx pgx.Tx, background bool, reserved map[int64]bool) ([]panda.GalleryRef, error) {
	q := s.q.WithTx(tx)
	// Never nil: a NULL array would make `<> ALL` exclude every row.
	excluded := make([]int64, 0, len(reserved))
	for id := range reserved {
		excluded = append(excluded, id)
	}
	now := time.Now()
	if !background {
		queued, err := q.PendingFetches(ctx, dbgen.PendingFetchesParams{Reserved: excluded, BatchLimit: panda.MaxBatchSize})
		if err != nil || len(queued) != 0 {
			return fetchRefs(queued), err
		}
		// Explicit jobs keep priority while they are reserved by other workers.
		if pending, err := q.HasPendingFetches(ctx); err != nil || pending {
			return nil, err
		}
		paused, err := q.MainBackgroundPaused(ctx)
		if err != nil || paused {
			return nil, err
		}
	}
	rows, err := q.PendingRefs(ctx, dbgen.PendingRefsParams{Now: now, Reserved: excluded, BatchLimit: panda.MaxBatchSize})
	if err != nil || len(rows) != 0 {
		return inventoryRefs(rows), err
	}
	// Proxy capacity can validate imports while other workers hold inventory.
	// The main fetcher retains its ordering until inventory is drained.
	if !background {
		if pending, err := q.HasPendingRefs(ctx, now); err != nil || pending {
			return nil, err
		}
	}
	return pendingImportRefs(ctx, tx, excluded)
}

func fetchRefs(rows []dbgen.PendingFetchesRow) []panda.GalleryRef {
	refs := make([]panda.GalleryRef, len(rows))
	for i, row := range rows {
		refs[i] = panda.GalleryRef{ID: row.GalleryID, Token: row.Token}
	}
	return refs
}

func inventoryRefs(rows []dbgen.PendingRefsRow) []panda.GalleryRef {
	refs := make([]panda.GalleryRef, len(rows))
	for i, row := range rows {
		refs[i] = panda.GalleryRef{ID: row.GalleryID, Token: row.Token}
	}
	return refs
}
