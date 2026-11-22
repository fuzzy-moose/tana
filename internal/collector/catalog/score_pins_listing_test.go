package catalog

import (
	"math"
	"slices"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// listedIDs returns the listed galleries above 100, leaving out the galleries
// of seedScoreGalleries.
func listedIDs(result collectorapi.CatalogResult) []int64 {
	ids := make([]int64, 0, len(result.Items))
	for _, item := range result.Items {
		if item.GalleryID > 100 {
			ids = append(ids, item.GalleryID)
		}
	}
	slices.Sort(ids)
	return ids
}

func assertListed(t *testing.T, result collectorapi.CatalogResult, want ...int64) {
	t.Helper()
	if got := listedIDs(result); !slices.Equal(got, want) {
		t.Fatalf("listed = %v, want %v", got, want)
	}
}

func assertScoreValue(t *testing.T, item collectorapi.CatalogItem, want float64) {
	t.Helper()
	if item.Score == nil || item.Score.Value == nil || math.Abs(*item.Score.Value-want) > 1e-9 {
		t.Fatalf("gallery %d score = %+v, want value %g", item.GalleryID, item.Score, want)
	}
}

// assertWeights compares an item's score weights, strongest first.
func assertWeights(t *testing.T, item collectorapi.CatalogItem, want []collectorapi.CatalogScoreWeight) {
	t.Helper()
	ok := item.Score != nil && len(item.Score.Weights) == len(want)
	for i := 0; ok && i < len(want); i++ {
		got := item.Score.Weights[i]
		ok = got.Kind == want[i].Kind && got.Label == want[i].Label && got.Source == want[i].Source &&
			math.Abs(got.Weight-want[i].Weight) <= 1e-9
	}
	if !ok {
		t.Fatalf("gallery %d weights = %+v, want %+v", item.GalleryID, item.Score, want)
	}
}

func pinnedTag(label string, weight float64) summaryTag {
	return summaryTag{label: label, weight: &weight, source: collectorapi.ScoreSourcePin}
}

func TestPinReplacesTheCalculatedWeightInTheNextPage(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	applyScoreModel(t, db, s, -5, storedScoreWeights()...)
	glasses, artist, hat := glassesWeight, artistWeight, hatWeight
	assertListed(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 102, 103)

	pin := setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", 0)
	page := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertListed(t, page, 101, 102, 103)
	assertScoreValue(t, itemByID(t, page, 101), 0)
	assertWeights(t, itemByID(t, page, 101), []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: 0, Source: collectorapi.ScoreSourcePin}})

	setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", 5)
	page = scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertListed(t, page, 101, 102, 103, 104)
	item := itemByID(t, page, 104)
	assertScoreValue(t, item, 5+6*artist)
	if math.Abs(5+6*artist-2.18) > 0.01 {
		t.Fatalf("score of 104 = %g, want about 2.18", 5+6*artist)
	}
	weights := []collectorapi.CatalogScoreWeight{{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: 5, Source: collectorapi.ScoreSourcePin}}
	ranked := []summaryTag{pinnedTag("female:glasses", 5)}
	for _, label := range scoreArtists[:5] {
		if len(weights) < scoreItemWeights {
			weights = append(weights, collectorapi.CatalogScoreWeight{Kind: collectorapi.ScoreKindTag, Label: label, Weight: artist})
		}
		ranked = append(ranked, weighted(label, artist))
	}
	assertWeights(t, item, weights)
	assertSummaryTags(t, item, ranked)

	hatPin := setPin(t, s, collectorapi.ScoreKindTag, "male:hat", -1)
	assertListed(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 101, 103, 104)
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertListed(t, hidden, 102)
	assertScore(t, itemByID(t, hidden, 102), collectorapi.ScoreRuleBelowThreshold)
	assertScoreValue(t, itemByID(t, hidden, 102), -1)

	for _, id := range []int64{pin.ID, hatPin.ID} {
		if err := s.RemoveScorePin(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	page = scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	assertScoreValue(t, itemByID(t, page, 101), glasses)
	assertScoreValue(t, itemByID(t, page, 102), hat)
	assertWeights(t, itemByID(t, page, 101), []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: glasses}})
	assertListed(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 102, 103)
}

func TestPinCountsForAValueWithoutACalculatedWeight(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	applyScoreModel(t, db, s, -5, storedScoreWeights()...)
	retainTagged(t, db, 105, "other:rare", "artist:u1")
	retain(t, db, panda.Metadata{ID: 106, Posted: 106, Uploader: "Someone", Category: "Doujinshi"}, true)
	retain(t, db, panda.Metadata{ID: 107, Posted: 107, Uploader: "Someone"}, true)
	assertListed(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 105, 106, 107)

	setPin(t, s, collectorapi.ScoreKindTag, "other:rare", -1)
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertListed(t, hidden, 105)
	item := itemByID(t, hidden, 105)
	assertScore(t, item, collectorapi.ScoreRuleBelowThreshold)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "other:rare", Weight: -1, Source: collectorapi.ScoreSourcePin}})

	setPin(t, s, collectorapi.ScoreKindTag, "other:rare", 0)
	item = itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 105)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "other:rare", Weight: 0, Source: collectorapi.ScoreSourcePin}})
	assertSummaryTags(t, item, []summaryTag{pinnedTag("other:rare", 0), {label: "artist:u1"}})

	setPin(t, s, collectorapi.ScoreKindUploader, "Someone", -1)
	assertListed(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), 106, 107)
	setPin(t, s, collectorapi.ScoreKindCategory, "Doujinshi", 2)
	page := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertListed(t, page, 105, 106)
	item = itemByID(t, page, 106)
	assertScoreValue(t, item, 1)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindCategory, Label: "doujinshi", Weight: 2, Source: collectorapi.ScoreSourcePin},
		{Kind: collectorapi.ScoreKindUploader, Label: "Someone", Weight: -1, Source: collectorapi.ScoreSourcePin}})
}

func TestPinsHaveNoEffectWithoutAnActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", -100)
	setPin(t, s, collectorapi.ScoreKindUploader, "Bad One", -100)
	assertNoPinEffect := func() {
		t.Helper()
		page := scorePage(t, s, "", collectorapi.CatalogOptions{})
		if page.ScoreFilter != nil {
			t.Fatalf("score filter = %+v", page.ScoreFilter)
		}
		if len(page.Items) != 14 {
			t.Fatalf("listed %d galleries, want 14", len(page.Items))
		}
		for _, item := range page.Items {
			if item.Score != nil {
				t.Fatalf("gallery %d score = %+v", item.GalleryID, item.Score)
			}
			for _, tag := range item.SummaryTags {
				if tag.Weight != nil || tag.Source != "" {
					t.Fatalf("gallery %d summary tag = %+v", item.GalleryID, tag)
				}
			}
		}
	}
	assertNoPinEffect()

	applyScoreModel(t, db, s, -5, storedScoreWeights()...)
	assertListed(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), 101, 104)
	if err := s.RemoveScoreModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertNoPinEffect()

	applyScoreModel(t, db, s, -5, storedScoreWeights()...)
	item := itemByID(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), 101)
	assertScoreValue(t, item, -100)
}
