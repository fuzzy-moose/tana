package catalog

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// RecentSampleSize is the number of the newest catalog galleries a preview
// classes by the hiding rules.
const RecentSampleSize = 2000

// recent classes the newest RecentSampleSize galleries that pass the
// predicate, in catalog order, by the first hiding rule that decides them:
// their verdict, a "never hide" override, an "always hide" override. The rest
// are counted by score under the candidate, when candidateID is set, and under
// the active model, when the filter has one.
func (f scoreFilter) recent(ctx context.Context, tx pgx.Tx, predicate string, args pgdb.Args, categories []string, candidateID *int64) (collectorapi.ScoreRecent, error) {
	var recent collectorapi.ScoreRecent
	args = slices.Clone(args)
	sample := catalogPageQuery(predicate, &args, categories, false, RecentSampleSize)
	never := f.matches(collectorapi.ScoreRuleNeverHide, &args)
	always := f.matches(collectorapi.ScoreRuleAlwaysHide, &args)
	candidateTenths, activeTenths := "NULL::integer", "NULL::integer"
	if candidateID != nil {
		candidateTenths = scoreTenths(args.Add(*candidateID)+"::bigint") + "::integer"
	}
	if f.model != nil {
		activeTenths = scoreTenths(args.Add(f.model.id)+"::bigint") + "::integer"
	}
	rows, err := tx.Query(ctx, `SELECT COALESCE(v.verdict, ''), `+never+`, `+always+`, `+candidateTenths+`, `+activeTenths+`
		FROM (`+sample+`) sample
		JOIN catalog_galleries g ON g.gallery_id = sample.gallery_id
		LEFT JOIN panda_gallery_verdicts v ON v.gallery_id = g.gallery_id`, args...)
	if err != nil {
		return recent, err
	}
	defer rows.Close()
	var candidateScores, activeScores []int
	for rows.Next() {
		var verdict string
		var isNever, isAlways bool
		var candidateScore, activeScore *int
		if err := rows.Scan(&verdict, &isNever, &isAlways, &candidateScore, &activeScore); err != nil {
			return recent, err
		}
		recent.Total++
		switch {
		case verdict == collectorapi.VerdictInteresting:
			recent.Interesting++
		case verdict == collectorapi.VerdictNotForMe:
			recent.NotForMe++
		case isNever:
			recent.NeverHidden++
		case isAlways:
			recent.AlwaysHidden++
		default:
			if candidateScore != nil {
				candidateScores = append(candidateScores, *candidateScore)
			}
			if activeScore != nil {
				activeScores = append(activeScores, *activeScore)
			}
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
