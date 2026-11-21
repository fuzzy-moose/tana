package catalog

import (
	"math"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func tenths(score float64) int {
	return int(math.Floor(score * 10))
}

func TestSuggestedThreshold(t *testing.T) {
	for _, tc := range []struct {
		name    string
		heldOut collectorapi.ScoreHeldOut
		want    int
	}{
		{"no scores", collectorapi.ScoreHeldOut{Total: 2, AlwaysHidden: 2, Scores: collectorapi.ScoreHistogram{Counts: []int{}}}, 0},
		{"at most 5 %", collectorapi.ScoreHeldOut{Total: 20, Scores: collectorapi.ScoreHistogram{FirstBucket: 0, Counts: []int{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 18}}}, 5},
		{"always hidden count", collectorapi.ScoreHeldOut{Total: 20, AlwaysHidden: 1, Scores: collectorapi.ScoreHistogram{FirstBucket: 0, Counts: []int{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 17}}}, 0},
		{"none hides at most 5 %", collectorapi.ScoreHeldOut{Total: 10, AlwaysHidden: 2, Scores: collectorapi.ScoreHistogram{FirstBucket: 3, Counts: []int{8}}}, 3},
	} {
		if got := suggestedThreshold(tc.heldOut); got != tc.want {
			t.Fatalf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}

// heldOutTags are the tags that favorites 1 to 4 and gallery 5 carry in
// seedHeldOutFavorites.
var heldOutTags = []string{"other:t1", "other:t2", "other:t3", "other:t4"}

// seedHeldOutFavorites retains a catalog of 20 galleries: favorites 1 to 4
// carry heldOutTags, which gallery 5 carries too; favorites 1 to 3 also carry
// other:three, and favorite 4 other:single. Each of heldOutTags has the
// held-out weight ln(13 / 10.75) from f - 1 = 3, F - 1 = 3, n = 5, N = 20;
// other:three has a candidate weight but none held out.
func seedHeldOutFavorites(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	retainRange(t, db, 1, 3, panda.Metadata{Tags: append([]string{"other:three"}, heldOutTags...)})
	retain(t, db, panda.Metadata{ID: 4, Posted: 4, Tags: append([]string{"other:single"}, heldOutTags...)}, true)
	retain(t, db, panda.Metadata{ID: 5, Posted: 5, Tags: heldOutTags}, true)
	retainRange(t, db, 6, 20, panda.Metadata{})
	favor(t, db, "42", 0, 1, 2, 3, 4)
}

var heldOutTagWeight = math.Log(13 / 10.75)

func heldOutCounts(t *testing.T, preview collectorapi.ScorePreview) map[int]int {
	t.Helper()
	if preview.Candidate == nil {
		t.Fatal("no candidate")
	}
	return histogramCounts(t, &preview.Candidate.HeldOut.Scores)
}

func TestPreviewHoldsOutEachFavorite(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	preview := previewScore(t, s)
	if _, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:three"); !ok {
		t.Fatalf("other:three has no candidate weight: %+v", preview.Candidate)
	}
	// The candidate weights would give buckets 9 and 11, a catalog count
	// without the favorite bucket 8, a favorite count F of 4 bucket 6.
	want := map[int]int{tenths(4 * heldOutTagWeight): 4}
	if got := heldOutCounts(t, preview); !reflect.DeepEqual(got, want) || tenths(4*heldOutTagWeight) != 7 {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
	heldOut := preview.Candidate.HeldOut
	if heldOut.Total != 4 || heldOut.AlwaysHidden != 0 || heldOut.NeverHidden != 0 {
		t.Fatalf("held out = %+v", heldOut)
	}
	if preview.Candidate.SuggestedThresholdTenths != 7 {
		t.Fatalf("suggested = %d", preview.Candidate.SuggestedThresholdTenths)
	}
}

func TestHeldOutScoresCountPins(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	setPin(t, s, collectorapi.ScoreKindTag, "other:three", 1)
	setPin(t, s, collectorapi.ScoreKindTag, "other:t1", 0)
	setPin(t, s, collectorapi.ScoreKindTag, "other:single", 2)
	three := 3 * heldOutTagWeight
	want := map[int]int{tenths(three + 1): 3, tenths(three + 2): 1}
	preview := previewScore(t, s)
	if got := heldOutCounts(t, preview); !reflect.DeepEqual(got, want) {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
	suggested := preview.Candidate.SuggestedThresholdTenths

	setPin(t, s, collectorapi.ScoreKindTag, "other:t2", 100)
	preview = previewScore(t, s)
	for bucket := range heldOutCounts(t, preview) {
		if bucket < 1000 {
			t.Fatalf("held-out bucket %d below 1000", bucket)
		}
	}
	candidate := preview.Candidate
	if candidate.SuggestedThresholdTenths <= suggested || candidate.SuggestedThresholdTenths < 1000 {
		t.Fatalf("suggested = %d, before %d", candidate.SuggestedThresholdTenths, suggested)
	}
	if err := s.ApplyScoreModel(t.Context(), candidate.ID, candidate.SuggestedThresholdTenths); err != nil {
		t.Fatal(err)
	}
}

func TestHeldOutScoresGiveTheAuthorBonusOnlyToAuthorsWithThreeFavorites(t *testing.T) {
	db, s := openCatalog(t)
	// artist:many on favorites 1 to 4 has the held-out weight ln(13 / 10.6);
	// artist:pair on favorites 1 and 2 has none.
	retainRange(t, db, 1, 2, panda.Metadata{Tags: []string{"artist:many", "artist:pair"}})
	retainRange(t, db, 3, 4, panda.Metadata{Tags: []string{"artist:many"}})
	retainRange(t, db, 5, 20, panda.Metadata{})
	favor(t, db, "42", 0, 1, 2, 3, 4)
	setFavoriteAuthor(t, db, "artist:many", 3)
	setFavoriteAuthor(t, db, "artist:pair", 2)
	setBonus(t, s, float(5))
	if got, want := heldOutCounts(t, previewScore(t, s)), map[int]int{50: 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
	if _, err := db.Exec(t.Context(), "UPDATE favorite_authors SET favorites = 2"); err != nil {
		t.Fatal(err)
	}
	if got, want := heldOutCounts(t, previewScore(t, s)), map[int]int{tenths(math.Log(13 / 10.6)): 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
	if _, err := db.Exec(t.Context(), "UPDATE favorite_authors fa SET favorites = 3 FROM catalog_tags t WHERE t.id = fa.tag_id AND t.value = 'many'"); err != nil {
		t.Fatal(err)
	}
	setPin(t, s, collectorapi.ScoreKindTag, "artist:many", -1)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:pair", 1)
	if got, want := heldOutCounts(t, previewScore(t, s)), map[int]int{0: 2, -10: 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
}

func TestHeldOutFavoritesDecidedByAnOverride(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Uploader: "Keep", Tags: []string{"other:drop", "other:a"}}, true)
	retain(t, db, panda.Metadata{ID: 2, Posted: 2, Tags: []string{"other:drop", "other:a"}}, true)
	retainRange(t, db, 3, 4, panda.Metadata{Tags: []string{"other:a"}})
	retain(t, db, panda.Metadata{ID: 5, Posted: 5, Expunged: true, Tags: []string{"other:a", "other:b"}}, true)
	retainRange(t, db, 6, 10, panda.Metadata{})
	favor(t, db, "42", 0, 1, 2, 3, 4, 5)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Keep", collectorapi.ScoreRuleNeverHide)
	setOverride(t, s, collectorapi.ScoreKindTag, "other:drop", collectorapi.ScoreRuleAlwaysHide)
	setPin(t, s, collectorapi.ScoreKindTag, "other:b", 1)

	candidate := previewScore(t, s).Candidate
	heldOut := candidate.HeldOut
	scored := 0
	for _, count := range heldOut.Scores.Counts {
		scored += count
	}
	if heldOut.Total != 5 || heldOut.NeverHidden != 1 || heldOut.AlwaysHidden != 1 || scored != 3 || heldOut.Total != heldOut.AlwaysHidden+heldOut.NeverHidden+scored {
		t.Fatalf("held out = %+v", heldOut)
	}
	// The override hides 1 of 5, more than 5 %, so the lowest scored bucket.
	if candidate.SuggestedThresholdTenths != heldOut.Scores.FirstBucket || heldOut.Scores.FirstBucket+len(heldOut.Scores.Counts)-1 < 10 {
		t.Fatalf("suggested %d, held out %+v", candidate.SuggestedThresholdTenths, heldOut)
	}
}

func TestHeldOutScoresBeyondTheWindowCountAtItsEdge(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"other:low"}}, true)
	retain(t, db, panda.Metadata{ID: 2, Posted: 2, Tags: []string{"other:high"}}, true)
	retain(t, db, panda.Metadata{ID: 3, Posted: 3}, true)
	favor(t, db, "42", 0, 1, 2, 3)
	setPin(t, s, collectorapi.ScoreKindTag, "other:low", -1e6)
	setPin(t, s, collectorapi.ScoreKindTag, "other:high", 1e12)
	preview := previewScore(t, s)
	want := map[int]int{-scoreWindowTenths: 1, 0: 1, scoreWindowTenths: 1}
	if got := heldOutCounts(t, preview); !reflect.DeepEqual(got, want) || len(preview.Candidate.HeldOut.Scores.Counts) > 2*scoreWindowTenths+1 {
		t.Fatalf("held-out counts = %v, want %v", got, want)
	}
}
