package catalog

import (
	"math"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// heldOutAuthorFavorites is the number of stored favorites an author needs to
// stay a favorite author without the held-out favorite, so that its tag gets
// the author bonus in a held-out score.
const heldOutAuthorFavorites = 3

// heldOutWeight is the weight of a value that a held-out favorite carries:
// the weight from the other favorites only, with the catalog counts
// unchanged.
func heldOutWeight(favorites, favoritesInScope, catalog, catalogInScope int) (float64, bool) {
	return scoreWeight(favorites-1, favoritesInScope-1, catalog, catalogInScope)
}

// heldOut scores each favorite in scope with the weights of the calculation
// held out from it, and counts it by the first rule that decides it: a "never
// hide" override, an "always hide" override, else its score. A value without a
// weight in the calculation has none held out either, so the weighted values'
// counts and the totals are all it reads. Pins count as in the listing; the
// tag of a favorite author gets the bonus only when the author has at least
// heldOutAuthorFavorites stored favorites.
func (f scoreFilter) heldOut(favorites []favoriteGallery, calculation scoreCalculation, authors []favoriteAuthor) collectorapi.ScoreHeldOut {
	rule := &weightRule{pins: pinnedWeights(f.pins), authorBonus: f.authorBonus, favoriteAuthors: map[int64]bool{}}
	for _, author := range authors {
		if author.favorites >= heldOutAuthorFavorites {
			rule.favoriteAuthors[author.tagID] = true
		}
	}
	calculated := func(value scoreValue) (float64, bool) {
		if _, ok := calculation.weights[value]; !ok {
			return 0, false
		}
		return heldOutWeight(calculation.favorites[value], calculation.favoritesInScope, calculation.catalog[value], calculation.catalogInScope)
	}
	heldOut := collectorapi.ScoreHeldOut{Total: len(favorites)}
	scores := make([]int, 0, len(favorites))
	for _, gallery := range favorites {
		if override, ok := f.decidingOverride(gallery.uploader, gallery.category, gallery.tagIDs); ok {
			if override.Rule == collectorapi.ScoreRuleNeverHide {
				heldOut.NeverHidden++
			} else {
				heldOut.AlwaysHidden++
			}
			continue
		}
		var score float64
		for _, value := range gallery.values() {
			weight, _ := rule.effective(value, calculated)
			score += weight
		}
		scores = append(scores, int(max(-scoreWindowTenths, min(scoreWindowTenths, math.Floor(score*10)))))
	}
	heldOut.Scores = *scoreHistogram(scores)
	return heldOut
}

// suggestedThreshold returns the highest threshold within the histogram's
// buckets that hides at most SuggestedFavoriteLoss of the held-out favorites,
// counting those an override always hides; the lowest bucket when none does,
// and 0 without scored favorites.
func suggestedThreshold(heldOut collectorapi.ScoreHeldOut) int {
	histogram := heldOut.Scores
	if len(histogram.Counts) == 0 {
		return 0
	}
	suggested := histogram.FirstBucket
	hidden := heldOut.AlwaysHidden
	for i, count := range histogram.Counts {
		if float64(hidden) <= SuggestedFavoriteLoss*float64(heldOut.Total) {
			suggested = histogram.FirstBucket + i
		}
		hidden += count
	}
	return suggested
}
