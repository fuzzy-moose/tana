package favorites

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db               *sql.DB
	q                *dbgen.Queries
	host, accountKey string
}

func (s *store) known(ctx context.Context, category int) (map[int64]int64, bool, error) {
	id, err := s.q.GetCategory(ctx, dbgen.GetCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	rows, err := s.q.KnownFavorites(ctx, id)
	if err != nil {
		return nil, false, err
	}
	known := make(map[int64]int64, len(rows))
	for _, row := range rows {
		known[row.GalleryID] = row.AddedAt
	}
	return known, true, nil
}

// complete opens the transaction only after all network work has succeeded.
// Readers keep the last committed snapshot throughout traversal and retries.
func (s *store) complete(ctx context.Context, category int, name string, full bool, entries []panda.Favorite) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	id, err := q.SaveCategory(ctx, dbgen.SaveCategoryParams{
		Host: s.host, AccountKey: s.accountKey, Category: int64(category), Name: name, SyncedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}
	if full {
		if err := q.DeleteFavorites(ctx, id); err != nil {
			return err
		}
	}
	for _, entry := range entries {
		if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token}); err != nil {
			return err
		}
		if err := q.SaveFavorite(ctx, dbgen.SaveFavoriteParams{
			CategoryID: id, GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token, AddedAt: entry.AddedAt.Unix(),
		}); err != nil {
			return err
		}
	}
	return tx.Commit()
}
