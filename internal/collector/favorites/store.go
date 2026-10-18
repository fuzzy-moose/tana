package favorites

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type store struct {
	db               *pgxpool.Pool
	q                *dbgen.Queries
	host, accountKey string
}

type job struct {
	dbgen.FavoriteSync
	category int
}

func (s *store) transaction(ctx context.Context, fn func(*dbgen.Queries) error) error {
	return s.withTx(ctx, func(_ pgx.Tx, q *dbgen.Queries) error { return fn(q) })
}

func (s *store) withTx(ctx context.Context, fn func(pgx.Tx, *dbgen.Queries) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx, s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// addedAt keeps whole seconds, the precision of Panda's favorite timestamps
// and of the stored incremental boundary they are compared with.
func addedAt(entry panda.Favorite) time.Time {
	return entry.AddedAt.Truncate(time.Second)
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
	return q.NewSync(ctx, dbgen.NewSyncParams{CategoryID: id, Full: full, QueuedAt: time.Now()})
}

func (s *store) enqueueCategories(ctx context.Context, q *dbgen.Queries, categories []int, full bool) error {
	for _, category := range categories {
		id, err := q.SaveCategory(ctx, dbgen.SaveCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
		if err != nil {
			return err
		}
		current, err := q.LockSync(ctx, id)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		switch current.State {
		case "running":
			if full && !current.Full {
				if err := q.QueueFullSync(ctx, id); err != nil {
					return err
				}
			}
			continue
		case "queued", "failed":
			if !full || current.Full {
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
		if err := newSync(ctx, q, id, full || collection.SyncedAt == nil || current.FollowupFull); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) next(ctx context.Context) (job, error) {
	var result job
	err := s.transaction(ctx, func(q *dbgen.Queries) error {
		var err error
		result, err = s.claim(ctx, q)
		return err
	})
	return result, err
}

// claim locks the next sync row and marks it running in q's transaction.
// Rows another claimer holds are skipped; pgx.ErrNoRows means no work.
func (s *store) claim(ctx context.Context, q *dbgen.Queries) (job, error) {
	row, err := q.NextSync(ctx, dbgen.NextSyncParams{Host: s.host, AccountKey: s.accountKey})
	if err != nil {
		return job{}, err
	}
	current := row.FavoriteSync
	if current.State == "success" || current.State == "failed" {
		if err := newSync(ctx, q, current.CategoryID, true); err != nil {
			return job{}, err
		}
	}
	// A row that left the claimable states meanwhile reports pgx.ErrNoRows.
	current, err = q.StartSync(ctx, dbgen.StartSyncParams{CategoryID: current.CategoryID, StartedAt: time.Now()})
	return job{FavoriteSync: current, category: int(row.Category)}, err
}

// errNotRunning reports a step whose sync left the running state meanwhile.
var errNotRunning = errors.New("favorite sync is no longer running")

// lockRunning locks a claimed sync row for the rest of a step's transaction.
func lockRunning(ctx context.Context, q *dbgen.Queries, id int64) (dbgen.FavoriteSync, error) {
	current, err := q.LockSync(ctx, id)
	if err == nil && current.State != "running" {
		err = errNotRunning
	}
	return current, err
}

func (s *store) known(ctx context.Context, category int) (map[int64]*time.Time, bool, error) {
	collection, err := s.q.GetCategory(ctx, dbgen.GetCategoryParams{Host: s.host, AccountKey: s.accountKey, Category: int64(category)})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	rows, err := s.q.KnownFavorites(ctx, collection.ID)
	if err != nil {
		return nil, false, err
	}
	known := make(map[int64]*time.Time, len(rows))
	for _, row := range rows {
		known[row.GalleryID] = row.CommittedAddedAt
	}
	return known, collection.SyncedAt != nil, nil
}

// A page, its public discoveries, and its continuation are one durable unit.
// Only successful traversal promotes the incremental boundary and removes membership.
func (s *store) savePage(ctx context.Context, current job, page panda.FavoritesPage, done bool, last *time.Time) error {
	return s.withTx(ctx, func(tx pgx.Tx, q *dbgen.Queries) error {
		baseline, err := q.LockDownloadSettings(ctx)
		if err != nil {
			return err
		}
		sync, err := lockRunning(ctx, q, current.CategoryID)
		if err != nil {
			return err
		}
		categories, err := q.DownloadCategories(ctx)
		if err != nil {
			return err
		}
		download := baseline == "ready" && slices.Contains(categories, int64(current.category))
		id := current.CategoryID
		if err := q.RenameCategory(ctx, dbgen.RenameCategoryParams{ID: id, Name: page.CategoryName}); err != nil {
			return err
		}
		for _, entry := range page.Entries {
			if err := q.SaveGalleryRef(ctx, dbgen.SaveGalleryRefParams{GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token}); err != nil {
				return err
			}
			if err := q.SaveFavorite(ctx, dbgen.SaveFavoriteParams{CategoryID: id, GalleryID: entry.GalleryRef.ID, Token: entry.GalleryRef.Token, AddedAt: addedAt(entry)}); err != nil {
				return err
			}
			if err := q.SaveSeenFavorite(ctx, dbgen.SaveSeenFavoriteParams{CategoryID: id, GalleryID: entry.GalleryRef.ID}); err != nil {
				return err
			}
			observed, err := q.ObserveFavorite(ctx, entry.GalleryRef.ID)
			if err != nil {
				return err
			}
			if observed != 0 && download {
				if err := downloads.EnqueueInTransaction(ctx, tx, entry.GalleryRef); err != nil {
					return err
				}
			}
		}
		if err := q.SaveVisitedPage(ctx, dbgen.SaveVisitedPageParams{CategoryID: id, Url: current.NextUrl}); err != nil {
			return err
		}
		at := time.Now()
		if err := q.SaveProgress(ctx, dbgen.SaveProgressParams{CategoryID: id, NextUrl: page.Next, LastSavedAt: &at, LastAddedAt: last}); err != nil {
			return err
		}
		if !done {
			return nil
		}
		if current.Full {
			if err := q.ReconcileFavorites(ctx, id); err != nil {
				return err
			}
		}
		if err := q.CommitFavorites(ctx, id); err != nil {
			return err
		}
		if err := q.CompleteCategory(ctx, dbgen.CompleteCategoryParams{ID: id, SyncedAt: &at}); err != nil {
			return err
		}
		if err := q.FinishSync(ctx, dbgen.FinishSyncParams{CategoryID: id, FinishedAt: &at}); err != nil {
			return err
		}
		if baseline == "collecting" && current.Full {
			// Admission may have queued a fresh full traversal while this page
			// was being fetched. Only that final traversal completes the baseline.
			if !sync.FollowupFull {
				if err := q.CompleteBaselineCategory(ctx, int64(current.category)); err != nil {
					return err
				}
			}
			return q.FinishDownloadBaseline(ctx)
		}
		return nil
	})
}

func (s *store) restart(ctx context.Context, id int64) error {
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		if _, err := lockRunning(ctx, q, id); err != nil {
			return err
		}
		if err := clearTraversal(ctx, q, id); err != nil {
			return err
		}
		return q.RestartTraversal(ctx, id)
	})
}

func (s *store) failed(ctx context.Context, current job, err error) error {
	at := time.Now()
	params := dbgen.FailSyncParams{CategoryID: current.CategoryID, State: "failed", LastError: err.Error(), LastErrorAt: &at, FinishedAt: &at}
	if retryable(err) {
		params.State = "running"
		params.FinishedAt = nil
		params.RetryAt = new(at.Add(panda.RetryDelay(current.Failures, err, at)))
	}
	return s.q.FailSync(ctx, params)
}
