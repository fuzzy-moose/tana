package catalog

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func previewWith(t *testing.T, s *Service, options collectorapi.ScorePreviewOptions) collectorapi.ScorePreview {
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

func tenths(score float64) int {
	return int(math.Floor(score * 10))
}

func TestPreviewWithoutAnActiveModelComparesWithNothing(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	preview := previewScore(t, s, false)
	if preview.Active != nil || preview.Evidence.NewVerdicts != nil || preview.Recent.Active != nil {
		t.Fatalf("preview = %+v", preview)
	}
	for _, weight := range preview.Candidate.Weights {
		if weight.ActiveWeight != nil {
			t.Fatalf("weight = %+v", weight)
		}
	}
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"active":null`, `"new_verdicts":null`, `"active_weight":null`} {
		if !strings.Contains(string(body), part) {
			t.Fatalf("preview JSON lacks %s: %s", part, body)
		}
	}
}

func TestPreviewDescribesTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	first := applyCandidate(t, s, -5)
	seedVerdicts(t, db, s, 11, 3, collectorapi.VerdictInteresting, "", "", "other:new")
	setVerdict(t, s, 1, collectorapi.VerdictInteresting)

	preview := previewScore(t, s, false)
	want := collectorapi.ScoreActive{CalculatedAt: first.Candidate.CalculatedAt, Favorites: false, ThresholdTenths: -5}
	if preview.Active == nil || *preview.Active != want {
		t.Fatalf("active = %+v, want %+v", preview.Active, want)
	}
	if got := preview.Evidence.NewVerdicts; got == nil || *got != 4 {
		t.Fatalf("new verdicts = %v", got)
	}
	glasses, ok := findWeight(preview, collectorapi.ScoreKindTag, "female:glasses")
	if !ok || glasses.ActiveWeight == nil || *glasses.ActiveWeight != weightOf(t, first, collectorapi.ScoreKindTag, "female:glasses") {
		t.Fatalf("female:glasses = %+v", glasses)
	}
	manga, ok := findWeight(preview, collectorapi.ScoreKindCategory, "manga")
	if !ok || manga.ActiveWeight == nil || *manga.ActiveWeight != weightOf(t, first, collectorapi.ScoreKindCategory, "manga") {
		t.Fatalf("manga = %+v", manga)
	}
	if added, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:new"); !ok || added.ActiveWeight != nil {
		t.Fatalf("other:new = %+v, %v", added, ok)
	}
}

func TestPreviewFavoritesFollowTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	retain(t, db, panda.Metadata{ID: 20, Posted: 20, Tags: []string{"male:hat"}}, true)
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_categories (id, host, account_key, category, name) VALUES (1, 'https://panda.test', '42', 1, 'One');
		INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES (1, 20, 'token20', now())`); err != nil {
		t.Fatal(err)
	}
	with := previewScore(t, s, true)
	if err := s.ApplyScoreModel(t.Context(), with.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	preview := previewWith(t, s, collectorapi.ScorePreviewOptions{})
	if !preview.Favorites || preview.Evidence.CountedFavorites != 1 || !preview.Active.Favorites || !preview.Candidate.Favorites {
		t.Fatalf("preview = %+v", preview)
	}
}

func TestPreviewRecentFollowsTheDefaultFilter(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Posted: 1, Category: "Manga"},
		{ID: 2, Posted: 2, Category: "Doujinshi"},
		{ID: 3, Posted: 3, Category: "Manga", Expunged: true},
		{ID: 4, Posted: 4, Category: "Manga", Tags: []string{"language:english"}},
		{ID: 5, Posted: 5, Category: "Manga", Uploader: "Drop"},
		{ID: 6, Posted: 6, Category: "Manga", Uploader: "Keep"},
	} {
		retain(t, db, entry, true)
	}
	setVerdict(t, s, 1, collectorapi.VerdictInteresting)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Drop", collectorapi.ScoreRuleAlwaysHide)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Keep", collectorapi.ScoreRuleNeverHide)
	for _, tc := range []struct {
		name    string
		options collectorapi.ScorePreviewOptions
		want    collectorapi.ScoreRecent
	}{
		{"none", collectorapi.ScorePreviewOptions{}, collectorapi.ScoreRecent{Total: 5, Interesting: 1, NeverHidden: 1, AlwaysHidden: 1}},
		{"category", collectorapi.ScorePreviewOptions{DefaultCategories: []string{"doujinshi"}}, collectorapi.ScoreRecent{Total: 1}},
		{"query", collectorapi.ScorePreviewOptions{DefaultQuery: "-l:english$"}, collectorapi.ScoreRecent{Total: 4, Interesting: 1, NeverHidden: 1, AlwaysHidden: 1}},
	} {
		preview := previewWith(t, s, tc.options)
		if preview.NoModel != collectorapi.ScoreNoNotForMe || !reflect.DeepEqual(preview.Recent, tc.want) {
			t.Fatalf("%s: no model %q, recent = %+v, want %+v", tc.name, preview.NoModel, preview.Recent, tc.want)
		}
	}
}

func TestPreviewRecentClassesGalleriesByTheHidingRules(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	for _, entry := range []panda.Metadata{
		{ID: 105, Posted: 105, Uploader: "Keep", Tags: []string{"female:glasses"}},
		{ID: 106, Posted: 106, Category: "Doujinshi"},
		{ID: 107, Posted: 107, Category: "Doujinshi", Uploader: "Keep"},
	} {
		retain(t, db, entry, true)
	}
	setOverride(t, s, collectorapi.ScoreKindUploader, "Keep", collectorapi.ScoreRuleNeverHide)
	setOverride(t, s, collectorapi.ScoreKindCategory, "doujinshi", collectorapi.ScoreRuleAlwaysHide)
	scored := func(preview collectorapi.ScorePreview) map[int]int {
		glasses := weightOf(t, preview, collectorapi.ScoreKindTag, "female:glasses")
		hat := weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat")
		artist := weightOf(t, preview, collectorapi.ScoreKindTag, "artist:a")
		counts := map[int]int{}
		for _, score := range []float64{glasses, hat, 0, glasses + 6*artist} {
			counts[tenths(score)]++
		}
		return counts
	}
	assertRecent := func(recent collectorapi.ScoreRecent, total, interesting int) {
		t.Helper()
		if recent.Total != total || recent.Interesting != interesting || recent.NotForMe != 5 || recent.NeverHidden != 2 || recent.AlwaysHidden != 1 {
			t.Fatalf("recent = %+v", recent)
		}
		sum := recent.Interesting + recent.NotForMe + recent.NeverHidden + recent.AlwaysHidden
		for _, count := range recent.Candidate.Counts {
			sum += count
		}
		if sum != recent.Total {
			t.Fatalf("recent = %+v adds up to %d", recent, sum)
		}
	}

	first := previewScore(t, s, false)
	assertRecent(first.Recent, 17, 5)
	if got, want := histogramCounts(t, first.Recent.Candidate), scored(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate = %v, want %v", got, want)
	}
	if want := map[int]int{-36: 1, -7: 1, 0: 1, 6: 1}; !reflect.DeepEqual(scored(first), want) {
		t.Fatalf("seeded scores = %v, want %v", scored(first), want)
	}
	if first.Recent.Active != nil {
		t.Fatalf("active = %+v", first.Recent.Active)
	}

	if err := s.ApplyScoreModel(t.Context(), first.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	seedVerdicts(t, db, s, 11, 3, collectorapi.VerdictInteresting, "", "", "female:glasses")
	second := previewScore(t, s, false)
	assertRecent(second.Recent, 20, 8)
	if got, want := histogramCounts(t, second.Recent.Candidate), scored(second); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate = %v, want %v", got, want)
	}
	if got, want := histogramCounts(t, second.Recent.Active), scored(first); !reflect.DeepEqual(got, want) {
		t.Fatalf("active = %v, want %v", got, want)
	}
	if reflect.DeepEqual(scored(first), scored(second)) {
		t.Fatalf("the new verdicts leave the scores as they were: %v", scored(first))
	}

	recent := second.Recent
	for threshold := -40; threshold <= 10; threshold++ {
		if err := s.SetScoreThreshold(t.Context(), threshold); err != nil {
			t.Fatal(err)
		}
		want := recent.NotForMe + recent.AlwaysHidden
		for i, count := range recent.Active.Counts {
			if recent.Active.FirstBucket+i < threshold {
				want += count
			}
		}
		if got := len(scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}).Items); got != want {
			t.Fatalf("threshold %d: listing hides %d, recent says %d (%+v)", threshold, got, want, recent)
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

func TestPreviewRecentCountsScoresBeyondTheWindowAtItsEdge(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	applyCandidate(t, s, 0)
	for tag, weight := range map[string]float64{"hat": 1e12, "glasses": -1001} {
		if _, err := db.Exec(t.Context(), `UPDATE panda_score_weights w SET weight = $1
			FROM panda_score_models m, catalog_tags tag
			WHERE m.active AND w.model_id = m.id AND w.tag_id = tag.id AND tag.value_lower = $2`, weight, tag); err != nil {
			t.Fatal(err)
		}
	}
	preview := previewScore(t, s, false)
	want := map[int]int{-scoreWindowTenths: 2, 0: 1, scoreWindowTenths: 1}
	if got := histogramCounts(t, preview.Recent.Active); !reflect.DeepEqual(got, want) {
		t.Fatalf("active counts = %v, want %v", got, want)
	}
	heldOut := preview.Candidate.HeldOut
	for _, histogram := range []*collectorapi.ScoreHistogram{preview.Recent.Active, preview.Recent.Candidate, &heldOut.Interesting.Scores, &heldOut.NotForMe.Scores, &heldOut.Neutral.Scores} {
		if len(histogram.Counts) > 2*scoreWindowTenths+1 {
			t.Fatalf("histogram has %d counts", len(histogram.Counts))
		}
	}
}
