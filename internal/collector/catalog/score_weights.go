package catalog

import (
	"math"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// Constants of the score model calculation.
const (
	// PriorStrength is the pseudo-count added to the favorites that carry a
	// value and to their expected number, which keeps a weight near zero while
	// few favorites carry the value.
	PriorStrength = 10
	// MinSupport is the number of favorites in scope a value needs for a
	// weight.
	MinSupport = 3
	// FewFavorites is the number of favorites in scope under which a preview
	// warns that its weights are unreliable.
	FewFavorites = 50
	// SuggestedFavoriteLoss is the largest share of the held-out favorites
	// that the suggested threshold hides.
	SuggestedFavoriteLoss = 0.05
)

// scoreValue names a tag by its catalog_tags ID, or an uploader or category by
// its text.
type scoreValue struct {
	kind  string
	tagID int64
	value string
}

// scoreWeight is the natural logarithm of the smoothed lift of a value that
// favorites of the F favorites in scope and catalog of the N catalog galleries
// in scope carry: how much more frequent it is in the favorites than in the
// catalog. A value that fewer than MinSupport favorites carry, or that is not
// more frequent in the favorites than in the catalog, gets no weight, so a
// weight is always above 0.
func scoreWeight(favorites, favoritesInScope, catalog, catalogInScope int) (float64, bool) {
	if favorites < MinSupport || catalogInScope == 0 {
		return 0, false
	}
	expected := float64(favoritesInScope) * float64(catalog) / float64(catalogInScope)
	weight := math.Log((float64(favorites) + PriorStrength) / (expected + PriorStrength))
	if !(weight > 0) {
		return 0, false
	}
	return weight, true
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
