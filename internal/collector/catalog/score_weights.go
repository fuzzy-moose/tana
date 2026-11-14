package catalog

import (
	"math"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// Constants of the score model calculation.
const (
	// NeutralWeight is how much a neutral gallery counts on the acceptable side,
	// relative to an "Interesting" gallery.
	NeutralWeight = 0.1
	// PriorStrength is the number of galleries of prior that keeps a weight near
	// zero while its own evidence is small.
	PriorStrength = 10
	// MinSupport is the number of evidence galleries a value needs for a weight.
	MinSupport = 3
	// FewVerdicts is the verdict count of either type under which a preview
	// warns that its weights are unreliable.
	FewVerdicts = 20
)

type scoreClass int

const (
	scoreInteresting scoreClass = iota
	scoreNotForMe
	scoreNeutral
)

// scoreCounts holds numbers of evidence galleries per class. Interesting
// includes counted favorites.
type scoreCounts struct {
	interesting, notForMe, neutral int
}

func (c *scoreCounts) add(class scoreClass) {
	switch class {
	case scoreInteresting:
		c.interesting++
	case scoreNotForMe:
		c.notForMe++
	case scoreNeutral:
		c.neutral++
	}
}

// scoreValue names a tag by its catalog_tags ID, or an uploader or category by
// its text.
type scoreValue struct {
	kind  string
	tagID int64
	value string
}

// evidenceGallery is one evidence gallery with its class and catalog values.
type evidenceGallery struct {
	galleryID int64
	class     scoreClass
	uploader  string
	category  string
	tagIDs    []int64
}

// tallyScoreCounts counts the galleries per class, and per class the galleries
// that carry each tag, each non-empty uploader and each non-empty category.
func tallyScoreCounts(evidence []evidenceGallery) (scoreCounts, map[scoreValue]scoreCounts) {
	var total scoreCounts
	values := map[scoreValue]scoreCounts{}
	count := func(value scoreValue, class scoreClass) {
		counts := values[value]
		counts.add(class)
		values[value] = counts
	}
	for _, gallery := range evidence {
		total.add(gallery.class)
		for _, tagID := range gallery.tagIDs {
			count(scoreValue{kind: collectorapi.ScoreKindTag, tagID: tagID}, gallery.class)
		}
		if gallery.uploader != "" {
			count(scoreValue{kind: collectorapi.ScoreKindUploader, value: gallery.uploader}, gallery.class)
		}
		if gallery.category != "" {
			count(scoreValue{kind: collectorapi.ScoreKindCategory, value: gallery.category}, gallery.class)
		}
	}
	return total, values
}

// scoreSides returns the acceptable and the "Not for me" side of the counts.
func scoreSides(c scoreCounts) (acceptable, rejected float64) {
	return float64(c.interesting) + NeutralWeight*float64(c.neutral), float64(c.notForMe)
}

// scoreModelPossible reports whether the evidence has both sides.
func scoreModelPossible(total scoreCounts) bool {
	acceptable, rejected := scoreSides(total)
	return acceptable > 0 && rejected > 0
}

// scoreWeight is the smoothed log odds ratio of the value's acceptable against
// its "Not for me" evidence, relative to all evidence. A value that fewer than
// MinSupport evidence galleries carry, or evidence without both sides, gives
// no weight.
func scoreWeight(total, value scoreCounts) (float64, bool) {
	if !scoreModelPossible(total) || value.interesting+value.notForMe+value.neutral < MinSupport {
		return 0, false
	}
	all, allRejected := scoreSides(total)
	acceptable, rejected := scoreSides(value)
	prior := all / (all + allRejected)
	return math.Log((acceptable+PriorStrength*prior)/(rejected+PriorStrength*(1-prior))) - math.Log(all/allRejected), true
}

// scoreWeights returns the weight of every value that gets one, or false when
// the evidence gives no model.
func scoreWeights(total scoreCounts, values map[scoreValue]scoreCounts) (map[scoreValue]float64, bool) {
	if !scoreModelPossible(total) {
		return nil, false
	}
	weights := map[scoreValue]float64{}
	for value, counts := range values {
		if weight, ok := scoreWeight(total, counts); ok {
			weights[value] = weight
		}
	}
	return weights, true
}
