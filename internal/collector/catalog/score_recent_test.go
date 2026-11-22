package catalog

import (
	"math"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// previewWith previews with the scope counts of the options settled, as the
// scope count worker leaves them.
func previewWith(t *testing.T, s *Service, options collectorapi.ScorePreviewOptions) collectorapi.ScorePreview {
	t.Helper()
	settleScope(t, s)
	preview := previewRaw(t, s, options)
	if preview.NoModel == collectorapi.ScoreCountsPreparing {
		settleScope(t, s)
		preview = previewRaw(t, s, options)
	}
	return preview
}

// previewRaw previews without waiting for the scope counts.
func previewRaw(t *testing.T, s *Service, options collectorapi.ScorePreviewOptions) collectorapi.ScorePreview {
	t.Helper()
	preview, err := s.PreviewScoreModel(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

// histogramCounts maps each non-empty bucket of the histogram to its count.
func histogramCounts(t *testing.T, histogram *collectorapi.ScoreHistogram) map[int]int {
	t.Helper()
	if histogram == nil {
		t.Fatal("histogram is nil")
	}
	counts := map[int]int{}
	for i, count := range histogram.Counts {
		if count > 0 {
			counts[histogram.FirstBucket+i] = count
		}
	}
	return counts
}

func TestPreviewRecentScoresUnderTheCandidateAndTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	manga := func(id int64, uploader string, tags ...string) {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Category: "Manga", Uploader: uploader, Tags: tags}, true)
	}
	for id := int64(1); id <= 3; id++ {
		manga(id, "", "other:a")
	}
	manga(4, "", "other:b")
	manga(5, "", "other:pinned")
	manga(6, "", "artist:x")
	manga(7, "", "other:high")
	manga(8, "", "other:low")
	manga(9, "Keep", "other:drop", "other:high")
	manga(10, "", "other:drop", "other:high")
	retain(t, db, panda.Metadata{ID: 11, Posted: 11, Category: "Manga", Expunged: true}, true)
	retain(t, db, panda.Metadata{ID: 12, Posted: 12, Category: "Doujinshi"}, true)
	favor(t, db, "42", 0, 1, 2, 3)
	applyScoreModel(t, db, s, 0, storedWeight{collectorapi.ScoreKindTag, "other:b", 1.5})
	setPin(t, s, collectorapi.ScoreKindTag, "other:pinned", 2)
	setPin(t, s, collectorapi.ScoreKindTag, "other:high", 1e12)
	setPin(t, s, collectorapi.ScoreKindTag, "other:low", -1e6)
	// The listing gives the bonus to a favorite author with 2 favorites.
	setFavoriteAuthor(t, db, "artist:x", 2)
	setBonus(t, s, float(3))

	recent := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: []string{"manga"}}).Recent
	if recent.Total != 10 {
		t.Fatalf("recent = %+v", recent)
	}
	// The scope of the candidate holds the expunged gallery 11: N = 11.
	// Galleries 7, 9 and 10 carry the pin beyond the window.
	edges := map[int]int{20: 1, 30: 1, scoreWindowTenths: 3, -scoreWindowTenths: 1}
	wantCandidate := map[int]int{tenths(math.Log(13 / (10 + 9/11.0))): 3, 0: 1}
	wantActive := map[int]int{0: 3, 15: 1}
	for bucket, count := range edges {
		wantCandidate[bucket], wantActive[bucket] = count, count
	}
	if got := histogramCounts(t, recent.Candidate); !reflect.DeepEqual(got, wantCandidate) {
		t.Fatalf("candidate = %v, want %v", got, wantCandidate)
	}
	if got := histogramCounts(t, recent.Active); !reflect.DeepEqual(got, wantActive) {
		t.Fatalf("active = %v, want %v", got, wantActive)
	}
	for _, histogram := range []*collectorapi.ScoreHistogram{recent.Candidate, recent.Active} {
		if len(histogram.Counts) > 2*scoreWindowTenths+1 {
			t.Fatalf("histogram has %d counts", len(histogram.Counts))
		}
	}
}

func TestScoreHistogram(t *testing.T) {
	for _, tc := range []struct {
		tenths []int
		want   collectorapi.ScoreHistogram
	}{
		{nil, collectorapi.ScoreHistogram{Counts: []int{}}},
		{[]int{3}, collectorapi.ScoreHistogram{FirstBucket: 3, Counts: []int{1}}},
		{[]int{0, -2, 0, 1}, collectorapi.ScoreHistogram{FirstBucket: -2, Counts: []int{1, 0, 2, 1}}},
	} {
		if got := scoreHistogram(tc.tenths); !reflect.DeepEqual(*got, tc.want) {
			t.Fatalf("%v: %+v, want %+v", tc.tenths, *got, tc.want)
		}
	}
}
