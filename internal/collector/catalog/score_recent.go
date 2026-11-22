package catalog

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// RecentSampleSize is the number of the newest catalog galleries a preview
// classes by the hiding rules.
const RecentSampleSize = 2000

// The histograms of a preview count scores from -1000.0 to 1000.0.
const scoreWindowTenths = 10000

// recent counts the newest RecentSampleSize galleries that pass the
// predicate, in catalog order, by score under the candidate, when candidateID
// is set, and under the active model, when the filter has one.
func (f scoreFilter) recent(ctx context.Context, tx pgx.Tx, predicate string, args pgdb.Args, categories []string, candidateID *int64) (collectorapi.ScoreRecent, error) {
	var recent collectorapi.ScoreRecent
	args = slices.Clone(args)
	sample := catalogPageQuery(predicate, &args, categories, false, RecentSampleSize)
	candidateTenths, activeTenths := "NULL::integer", "NULL::integer"
	if candidateID != nil {
		candidateTenths = windowTenths(f.scoreTenths(args.Add(*candidateID) + "::bigint"))
	}
	if f.model != nil {
		activeTenths = windowTenths(f.scoreTenths(args.Add(f.model.id) + "::bigint"))
	}
	rows, err := tx.Query(ctx, `SELECT `+candidateTenths+`, `+activeTenths+`
		FROM (`+sample+`) sample
		JOIN catalog_galleries g ON g.gallery_id = sample.gallery_id`, args...)
	if err != nil {
		return recent, err
	}
	defer rows.Close()
	var candidateScores, activeScores []int
	for rows.Next() {
		var candidateScore, activeScore *int
		if err := rows.Scan(&candidateScore, &activeScore); err != nil {
			return recent, err
		}
		recent.Total++
		if candidateScore != nil {
			candidateScores = append(candidateScores, *candidateScore)
		}
		if activeScore != nil {
			activeScores = append(activeScores, *activeScore)
		}
	}
	if err := rows.Err(); err != nil {
		return recent, err
	}
	if candidateID != nil {
		recent.Candidate = scoreHistogram(candidateScores)
	}
	if f.model != nil {
		recent.Active = scoreHistogram(activeScores)
	}
	return recent, nil
}

// windowTenths limits the SQL expression for a score's tenths to the window,
// as an integer.
func windowTenths(tenths string) string {
	return fmt.Sprintf("least(greatest(%s, %d), %d)::integer", tenths, -scoreWindowTenths, scoreWindowTenths)
}

// scoreHistogram counts the scores in tenths by bucket, from the lowest score
// to the highest. Without scores its counts are empty.
func scoreHistogram(tenths []int) *collectorapi.ScoreHistogram {
	histogram := &collectorapi.ScoreHistogram{Counts: []int{}}
	if len(tenths) == 0 {
		return histogram
	}
	histogram.FirstBucket = slices.Min(tenths)
	histogram.Counts = make([]int, slices.Max(tenths)-histogram.FirstBucket+1)
	for _, score := range tenths {
		histogram.Counts[score-histogram.FirstBucket]++
	}
	return histogram
}
