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

type job struct {
	dbgen.FavoriteSync
	category int
}

func (s *store) transaction(ctx context.Context, fn func(*dbgen.Queries) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit()
}

func clearTraversal(ctx context.Context, q *dbgen.Queries, id int64) error {
	if err := q.ClearSeenFavorites(ctx, id); err != nil {
		return err
	}
	return q.ClearVisitedPages(ctx, id)
}

func newSync(ctx context.Context, q *dbgen.Queries, id int64, full bool) error {
	if err := clearTraversal(ctx, q, id); err != nil {
		return err
	}
	var mode int64
	if full {
		mode = 1
	}
	return q.NewSync(ctx, dbgen.NewSyncParams{CategoryID: id, Full: mode, QueuedAt: time.Now().UnixMilli()})
}

// Admission and coalescing commit before the HTTP request is accepted.
func (s *store) enqueue(ctx context.Context, categories []int, full bool) error {
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		for _, category := range categories {
			id, err := q.SaveCategory(ctx, dbgen.SaveCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
			if err != nil {
				return err
			}
			current, err := q.GetSync(ctx, id)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			switch current.State {
			case "running":
				if full && current.Full == 0 {
					if err := q.QueueFullSync(ctx, id); err != nil {
						return err
					}
				}
				continue
			case "queued", "failed":
				if !full || current.Full != 0 {
					if current.State == "failed" {
						if err := q.ResumeSync(ctx, id); err != nil {
							return err
						}
					}
					continue
				}
			}
			collection, err := q.GetCategory(ctx, dbgen.GetCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
			if err != nil {
				return err
			}
			if err := newSync(ctx, q, id, full || collection.SyncedAt == 0 || current.FollowupFull != 0); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *store) next(ctx context.Context) (job, error) {
	var result job
	err := s.transaction(ctx, func(q *dbgen.Queries) error {
		row, err := q.NextSync(ctx, dbgen.NextSyncParams{Host: s.host, AccountKey: s.accountKey})
		if err != nil {
			return err
		}
		current := row.FavoriteSync
		if current.State == "success" || current.State == "failed" {
			if err := newSync(ctx, q, current.CategoryID, true); err != nil {
				return err
			}
		}
		if err := q.StartSync(ctx, dbgen.StartSyncParams{CategoryID: current.CategoryID, StartedAt: time.Now().UnixMilli()}); err != nil {
			return err
		}
		current, err = q.GetSync(ctx, current.CategoryID)
		result = job{FavoriteSync: current, category: int(row.Category)}
		return err
	})
	return result, err
}

func (s *store) known(ctx context.Context, category int) (map[int64]int64, bool, error) {
	collection, err := s.q.GetCategory(ctx, dbgen.GetCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	rows, err := s.q.KnownFavorites(ctx, collection.ID)
	if err != nil {
		return nil, false, err
	}
	known := make(map[int64]int64, len(rows))
	for _, row := range rows {
		known[row.GalleryID] = row.CommittedAddedAt.Int64
	}
	return known, collection.SyncedAt != 0, nil
}

// A page, its public discoveries, and its continuation are one durable unit.
// Only successful traversal promotes the incremental boundary and removes membership.
func (s *store) savePage(ctx context.Context, current job, page panda.FavoritesPage, done bool, last int64) error {
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		id := current.CategoryID
		if err := q.RenameCategory(ctx, dbgen.RenameCategoryParams{ID: id, Name: page.CategoryName}); err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token}); err != nil {
				return err
			}
			if err := q.SaveFavorite(ctx, dbgen.SaveFavoriteParams{CategoryID: id, GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token, AddedAt: entry.AddedAt.Unix()}); err != nil {
				return err
			}
			if err := q.SaveSeenFavorite(ctx, dbgen.SaveSeenFavoriteParams{CategoryID: id, GalleryID: entry.GalleryRef.ID}); err != nil {
				return err
			}
		}
		if err := q.SaveVisitedPage(ctx, dbgen.SaveVisitedPageParams{CategoryID: id, Url: current.NextUrl}); err != nil {
			return err
		}
		at := time.Now().UnixMilli()
		if err := q.SaveProgress(ctx, dbgen.SaveProgressParams{CategoryID: id, NextUrl: page.Next, LastSavedAt: at, LastAddedAt: last}); err != nil {
			return err
		}
		if !done {
			return nil
		}
		if current.Full != 0 {
			if err := q.ReconcileFavorites(ctx, id); err != nil {
				return err
			}
		}
		if err := q.CommitFavorites(ctx, id); err != nil {
			return err
		}
		if err := q.CompleteCategory(ctx, dbgen.CompleteCategoryParams{ID: id, SyncedAt: at}); err != nil {
			return err
		}
		return q.FinishSync(ctx, dbgen.FinishSyncParams{CategoryID: id, FinishedAt: at})
	})
}

func (s *store) restart(ctx context.Context, id int64) error {
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		if err := clearTraversal(ctx, q, id); err != nil {
			return err
		}
		return q.RestartTraversal(ctx, id)
	})
}

func (s *store) failed(ctx context.Context, current job, err error) error {
	at := time.Now()
	params := dbgen.FailSyncParams{CategoryID: current.CategoryID, State: "failed", LastError: err.Error(), LastErrorAt: at.UnixMilli(), FinishedAt: at.UnixMilli()}
	if retryable(err) {
		params.State = "running"
		params.FinishedAt = 0
		params.RetryAt = at.Add(panda.RetryDelay(current.Failures, err, at)).UnixMilli()
	}
	return s.q.FailSync(ctx, params)
}
