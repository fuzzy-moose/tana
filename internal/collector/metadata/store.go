package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db *sql.DB
	q  *dbgen.Queries
}

// complete commits successful metadata and related references together, so a
// restart cannot lose discovered work after marking its source collected.
func (s *store) complete(ctx context.Context, entries []panda.Metadata, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	for _, entry := range entries {
		if entry.Error == "" {
			body, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			if err := q.SaveMetadata(ctx, dbgen.SaveMetadataParams{GalleryID: entry.ID, Body: body, RefreshedAt: at.UnixMilli()}); err != nil {
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
			GalleryID: entry.ID, MetadataAttemptedAt: sql.NullInt64{Int64: at.UnixMilli(), Valid: true},
			MetadataError: sql.NullString{String: entry.Error, Valid: entry.Error != ""},
		}); err != nil {
			return err
		}
	}
	if err := q.RecordBatchSuccess(ctx, sql.NullInt64{Int64: at.UnixMilli(), Valid: true}); err != nil {
		return err
	}
	return tx.Commit()
}
