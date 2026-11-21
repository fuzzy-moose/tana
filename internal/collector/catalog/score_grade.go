package catalog

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// writeScoreInputs runs a write of a pin, an override or the author bonus,
// which can change the held-out favorite scores of the active model, and
// stores them again in its transaction.
func (s *Service) writeScoreInputs(ctx context.Context, write func(tx pgx.Tx) error) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := write(tx); err != nil {
		return err
	}
	if err := storeFavoriteScores(ctx, tx, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RefreshFavoriteScores stores the held-out favorite scores of the active
// model again, for a model active before they were stored and for favorites
// synced since the last score write.
func (s *Service) RefreshFavoriteScores(ctx context.Context) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := storeFavoriteScores(ctx, tx, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// storeFavoriteScores calculates the held-out favorite scores of the active
// model and stores them with its ID and scope. It locks the row before it
// reads (ADR 0018), so that of two concurrent writes the later one calculates
// with both. The scope is the one stored with the model; applying takes the
// scope of the counts, which the candidate was calculated from, and so does a
// model without stored scores, the best guess for it.
func storeFavoriteScores(ctx context.Context, tx pgx.Tx, applying bool) error {
	var storedModel *int64
	var scope catalogScope
	if err := tx.QueryRow(ctx, "SELECT model_id, query, categories FROM panda_favorite_scores WHERE id = 1 FOR UPDATE").
		Scan(&storedModel, &scope.query, &scope.categories); err != nil {
		return err
	}
	filter, err := readScoreFilter(ctx, tx)
	if err != nil {
		return err
	}
	scores := []int{}
	var modelID *int64
	if filter.model != nil {
		modelID = &filter.model.id
		if applying || storedModel == nil || *storedModel != filter.model.id {
			if scope, err = readScope(ctx, tx, ""); err != nil {
				return err
			}
		}
		if scores, err = filter.favoriteScores(ctx, tx, scope); err != nil {
			return err
		}
	} else {
		scope = catalogScope{categories: []string{}}
	}
	_, err = tx.Exec(ctx, "UPDATE panda_favorite_scores SET model_id = $1, scores = $2, query = $3, categories = $4 WHERE id = 1",
		modelID, scores, scope.query, scope.categories)
	return err
}

// favoriteScores calculates the held-out favorite scores of the active model
// in tenths, lowest first: the favorites in the scope, scored with the
// model's stored counts under the current pins, overrides and author bonus,
// like a measurement of a candidate. Without a scope it returns none.
func (f scoreFilter) favoriteScores(ctx context.Context, tx pgx.Tx, counted catalogScope) ([]int, error) {
	scores := []int{}
	if counted.query == nil {
		return scores, nil
	}
	scope, scopeArgs, _, err := catalogPredicate(ctx, tx, counted.options())
	if err != nil {
		return scores, err
	}
	favorites, err := favoritesInScope(ctx, tx, scope, scopeArgs)
	if err != nil || len(favorites) == 0 {
		return scores, err
	}
	calculation, _, ok, err := readCalculation(ctx, tx, f.model.id, true)
	if err != nil || !ok {
		return scores, err
	}
	authors, err := favoriteAuthors(ctx, tx)
	if err != nil {
		return scores, err
	}
	scores, _, _ = f.heldOutScores(favorites, calculation, authors)
	slices.Sort(scores)
	return scores, nil
}

// markGrades gives each item with a score the number of held-out favorite
// scores above it and its score grade. A score on a cut point gets the higher
// grade: the top quarter holds a score that fewer than a quarter of the
// favorite scores exceed.
func markGrades(items []collectorapi.CatalogItem, favorites []int) {
	if len(favorites) == 0 {
		return
	}
	for i := range items {
		score := items[i].Score
		if score == nil || score.Value == nil {
			continue
		}
		tenths := scoreTenthsInWindow(*score.Value)
		// The first favorite score at or above tenths + 1 is the first above.
		firstAbove, _ := slices.BinarySearch(favorites, tenths+1)
		above := len(favorites) - firstAbove
		score.FavoritesAbove = &above
		switch {
		case 4*above < len(favorites):
			score.Grade = collectorapi.ScoreGradeTopQuarter
		case 2*above < len(favorites):
			score.Grade = collectorapi.ScoreGradeTopHalf
		}
	}
}
