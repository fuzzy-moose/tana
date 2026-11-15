package catalog

import (
	"context"
	"math"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// SuggestedInterestingLoss is the largest share of the held-out "Interesting"
// galleries that the suggested threshold hides.
const SuggestedInterestingLoss = 0.05

// heldOutFolds is the number of folds of the held-out scores: an evidence
// gallery is in fold gallery_id % heldOutFolds.
const heldOutFolds = 5

// heldOutTenths scores each evidence gallery, as floor(score * 10) limited to
// the window of the histograms, with the effective weights of its fold under
// the rule, whose calculated weights are those from the evidence outside the
// fold. total and values are the counts of all the evidence. A fold whose
// training evidence lacks one side calculates no weight, so only the pins and
// the author bonus score its galleries.
func heldOutTenths(evidence []evidenceGallery, total scoreCounts, values map[scoreValue]scoreCounts, rule *weightRule) map[int64]int {
	var folds [heldOutFolds][]evidenceGallery
	for _, gallery := range evidence {
		fold := gallery.galleryID % heldOutFolds
		folds[fold] = append(folds[fold], gallery)
	}
	scores := make(map[int64]int, len(evidence))
	for _, fold := range folds {
		foldTotal, foldValues := tallyScoreCounts(fold)
		train := total.minus(foldTotal)
		calculated := func(value scoreValue) (float64, bool) {
			return scoreWeight(train, values[value].minus(foldValues[value]))
		}
		weight := func(value scoreValue) float64 {
			w, _ := rule.effective(value, calculated)
			return w
		}
		for _, gallery := range fold {
			var score float64
			for _, tagID := range gallery.tagIDs {
				score += weight(scoreValue{kind: collectorapi.ScoreKindTag, tagID: tagID})
			}
			if gallery.uploader != "" {
				score += weight(scoreValue{kind: collectorapi.ScoreKindUploader, value: gallery.uploader})
			}
			if gallery.category != "" {
				score += weight(scoreValue{kind: collectorapi.ScoreKindCategory, value: gallery.category})
			}
			scores[gallery.galleryID] = int(max(-scoreWindowTenths, min(scoreWindowTenths, math.Floor(score*10))))
		}
	}
	return scores
}

// weightRule holds what the weight rule needs besides the calculated
// weights: the pins, the author bonus (nil while it is off) and the
// catalog_tags IDs of the favorite authors. A nil rule has none of them.
type weightRule struct {
	pins            map[scoreValue]float64
	authorBonus     *float64
	favoriteAuthors map[int64]bool
}

// effective is the Go side of the weight rule: the pin of the value, else for
// the tag of a favorite author while the bonus is on the larger of its
// calculated weight and the bonus, else its calculated weight. ok is false
// when it has none.
func (r *weightRule) effective(value scoreValue, calculated func(scoreValue) (float64, bool)) (weight float64, ok bool) {
	if r == nil {
		return calculated(value)
	}
	if pin, ok := r.pins[value]; ok {
		return pin, true
	}
	weight, ok = calculated(value)
	if r.authorBonus != nil && value.kind == collectorapi.ScoreKindTag && r.favoriteAuthors[value.tagID] && (!ok || weight < *r.authorBonus) {
		return *r.authorBonus, true
	}
	return weight, ok
}

func (c scoreCounts) minus(other scoreCounts) scoreCounts {
	return scoreCounts{interesting: c.interesting - other.interesting, notForMe: c.notForMe - other.notForMe, neutral: c.neutral - other.neutral}
}

// heldOut classes the evidence galleries that pass the predicate by the
// override rules, a "never hide" match first, and counts the held-out scores
// of the rest per class. The verdict decides nothing here.
func (f scoreFilter) heldOut(ctx context.Context, tx pgx.Tx, predicate string, args pgdb.Args, evidence []evidenceGallery, tenths map[int64]int) (collectorapi.ScoreHeldOut, error) {
	var heldOut collectorapi.ScoreHeldOut
	ids := make([]int64, 0, len(evidence))
	for _, gallery := range evidence {
		ids = append(ids, gallery.galleryID)
	}
	args = slices.Clone(args)
	rows, err := tx.Query(ctx, "SELECT g.gallery_id FROM catalog_galleries g WHERE g.gallery_id = ANY("+args.Add(ids)+"::bigint[]) AND "+predicate, args...)
	if err != nil {
		return heldOut, err
	}
	passes, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return heldOut, err
	}
	passing := make(map[int64]bool, len(passes))
	for _, id := range passes {
		passing[id] = true
	}
	effects := map[scoreClass]*collectorapi.ScoreClassEffect{
		scoreInteresting: &heldOut.Interesting, scoreNotForMe: &heldOut.NotForMe, scoreNeutral: &heldOut.Neutral,
	}
	scores := map[scoreClass][]int{}
	for _, gallery := range evidence {
		if !passing[gallery.galleryID] {
			continue
		}
		effect := effects[gallery.class]
		effect.Total++
		override, ok := f.decidingOverride(gallery.uploader, gallery.category, gallery.tagIDs)
		switch {
		case ok && override.Rule == collectorapi.ScoreRuleNeverHide:
			effect.NeverHidden++
		case ok:
			effect.AlwaysHidden++
		default:
			scores[gallery.class] = append(scores[gallery.class], tenths[gallery.galleryID])
		}
	}
	for class, effect := range effects {
		effect.Scores = *scoreHistogram(scores[class])
	}
	return heldOut, nil
}

// suggestedThreshold returns the highest threshold within the histogram's
// buckets that hides at most SuggestedInterestingLoss of the class's
// galleries, counting those an override always hides; the first bucket when
// none does, and 0 without scored galleries.
func suggestedThreshold(effect collectorapi.ScoreClassEffect) int {
	histogram := effect.Scores
	if len(histogram.Counts) == 0 {
		return 0
	}
	suggested := histogram.FirstBucket
	hidden := effect.AlwaysHidden
	for i, count := range histogram.Counts {
		if float64(hidden) <= SuggestedInterestingLoss*float64(effect.Total) {
			suggested = histogram.FirstBucket + i
		}
		hidden += count
	}
	return suggested
}
