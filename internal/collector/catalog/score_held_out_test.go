package catalog

import (
	"reflect"
	"slices"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func histogramSum(histogram collectorapi.ScoreHistogram) int {
	sum := 0
	for _, count := range histogram.Counts {
		sum += count
	}
	return sum
}

// scoredBuckets lists the bucket of every scored gallery of the histogram,
// lowest first.
func scoredBuckets(histogram collectorapi.ScoreHistogram) []int {
	var buckets []int
	for i, count := range histogram.Counts {
		for range count {
			buckets = append(buckets, histogram.FirstBucket+i)
		}
	}
	return buckets
}

func TestSuggestedThreshold(t *testing.T) {
	for _, tc := range []struct {
		name   string
		effect collectorapi.ScoreClassEffect
		want   int
	}{
		{"empty", collectorapi.ScoreClassEffect{Total: 3, AlwaysHidden: 3, Scores: collectorapi.ScoreHistogram{Counts: []int{}}}, 0},
		// 40 galleries: the threshold -9 hides 2 (5 %), -8 hides 3.
		{"loss limit", collectorapi.ScoreClassEffect{Total: 40, Scores: collectorapi.ScoreHistogram{FirstBucket: -10, Counts: []int{2, 1, 0, 37}}}, -9},
		{"always hidden counts", collectorapi.ScoreClassEffect{Total: 40, AlwaysHidden: 1, Scores: collectorapi.ScoreHistogram{FirstBucket: -10, Counts: []int{1, 1, 0, 37}}}, -9},
		{"none meets", collectorapi.ScoreClassEffect{Total: 10, AlwaysHidden: 1, Scores: collectorapi.ScoreHistogram{FirstBucket: 4, Counts: []int{5, 4}}}, 4},
		{"highest bucket", collectorapi.ScoreClassEffect{Total: 100, NeverHidden: 98, Scores: collectorapi.ScoreHistogram{FirstBucket: 2, Counts: []int{1, 0, 1}}}, 4},
	} {
		if got := suggestedThreshold(tc.effect); got != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestHeldOutCountsEvidenceThatPassesTheDefaultFilter(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	for _, entry := range []panda.Metadata{
		{ID: 11, Posted: 11},
		{ID: 12, Posted: 12, Category: "Doujinshi"},
		{ID: 13, Posted: 13, Category: "Doujinshi", Tags: []string{"male:hat"}},
		{ID: 14, Posted: 14, Uploader: "Bad One"},
		{ID: 15, Posted: 15, Expunged: true, Tags: []string{"male:hat"}},
	} {
		retain(t, db, entry, true)
	}
	if err := s.RecordDisplays(t.Context(), []int64{11, 12}, ""); err != nil {
		t.Fatal(err)
	}
	setVerdict(t, s, 13, collectorapi.VerdictInteresting)
	setVerdict(t, s, 14, collectorapi.VerdictNotForMe)
	setVerdict(t, s, 15, collectorapi.VerdictInteresting)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Bad One", collectorapi.ScoreRuleAlwaysHide)
	setOverride(t, s, collectorapi.ScoreKindTag, "artist:a", collectorapi.ScoreRuleNeverHide)
	setOverride(t, s, collectorapi.ScoreKindCategory, "manga", collectorapi.ScoreRuleNeverHide)

	type counts struct{ total, always, never, scored int }
	effectCounts := func(effect collectorapi.ScoreClassEffect) counts {
		return counts{effect.Total, effect.AlwaysHidden, effect.NeverHidden, histogramSum(effect.Scores)}
	}
	for _, tc := range []struct {
		name                           string
		categories                     []string
		interesting, notForMe, neutral counts
	}{
		// "Not for me": 1 to 3 match both overrides, 14 only "always hide",
		// 4 and 5 none. "Interesting": 6 to 8 in manga, 9, 10 and 13 scored,
		// 15 expunged, which still counts in the weights. Neutral: 11 and 12.
		{"none", nil, counts{6, 0, 3, 3}, counts{6, 1, 3, 2}, counts{2, 0, 0, 2}},
		{"doujinshi", []string{"doujinshi"}, counts{1, 0, 0, 1}, counts{}, counts{1, 0, 0, 1}},
	} {
		preview := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: tc.categories})
		if preview.Candidate == nil {
			t.Fatalf("%s: preview = %+v", tc.name, preview)
		}
		heldOut := preview.Candidate.HeldOut
		got := []counts{effectCounts(heldOut.Interesting), effectCounts(heldOut.NotForMe), effectCounts(heldOut.Neutral)}
		if want := []counts{tc.interesting, tc.notForMe, tc.neutral}; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: held out = %+v, want %+v", tc.name, got, want)
		}
		if hat, ok := findWeight(preview, collectorapi.ScoreKindTag, "male:hat"); !ok || hat.Interesting != 7 {
			t.Fatalf("%s: male:hat = %+v, %v", tc.name, hat, ok)
		}
	}
}

func TestHeldOutScoresIgnoreATagOfOneFold(t *testing.T) {
	db, s := openCatalog(t)
	seedVerdicts(t, db, s, 1, 9, collectorapi.VerdictNotForMe, "", "", "female:glasses")
	seedVerdicts(t, db, s, 11, 9, collectorapi.VerdictInteresting, "", "", "male:hat")
	foldTags := []string{"female:glasses", "other:fold"}
	seedVerdicts(t, db, s, 20, 1, collectorapi.VerdictNotForMe, "", "", foldTags...)
	seedVerdicts(t, db, s, 25, 1, collectorapi.VerdictNotForMe, "", "", foldTags...)
	seedVerdicts(t, db, s, 30, 1, collectorapi.VerdictInteresting, "", "", "male:hat", "other:fold")

	with := previewScore(t, s, false)
	if weight := weightOf(t, with, collectorapi.ScoreKindTag, "other:fold"); weight == 0 {
		t.Fatal("other:fold has no weight in the candidate")
	}
	for _, id := range []int64{20, 25} {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: []string{"female:glasses"}}, true)
	}
	retain(t, db, panda.Metadata{ID: 30, Posted: 30, Tags: []string{"male:hat"}}, true)
	without := previewScore(t, s, false)
	if _, ok := findWeight(without, collectorapi.ScoreKindTag, "other:fold"); ok {
		t.Fatal("other:fold still has a weight")
	}
	if !reflect.DeepEqual(with.Candidate.HeldOut, without.Candidate.HeldOut) {
		t.Fatalf("held out with the fold tag %+v, without %+v", with.Candidate.HeldOut, without.Candidate.HeldOut)
	}
}

func TestHeldOutScoresAFoldWithoutTrainingSideAtZero(t *testing.T) {
	db, s := openCatalog(t)
	tags := []string{"female:glasses"}
	for _, id := range []int64{5, 10, 15} {
		seedVerdicts(t, db, s, id, 1, collectorapi.VerdictNotForMe, "", "", tags...)
	}
	seedVerdicts(t, db, s, 1, 4, collectorapi.VerdictInteresting, "", "", "male:hat")
	preview := previewScore(t, s, false)
	if weightOf(t, preview, collectorapi.ScoreKindTag, "female:glasses") == 0 {
		t.Fatal("female:glasses has no weight in the candidate")
	}
	want := collectorapi.ScoreHistogram{FirstBucket: 0, Counts: []int{3}}
	if got := preview.Candidate.HeldOut.NotForMe.Scores; !reflect.DeepEqual(got, want) {
		t.Fatalf("not for me scores = %+v, want %+v", got, want)
	}
}

func TestHeldOutScoresSeparateTheClasses(t *testing.T) {
	db, s := openCatalog(t)
	seedVerdicts(t, db, s, 1, 10, collectorapi.VerdictNotForMe, "", "", "female:glasses")
	seedVerdicts(t, db, s, 11, 10, collectorapi.VerdictInteresting, "", "", "male:hat")
	preview := previewScore(t, s, false)
	heldOut := preview.Candidate.HeldOut
	rejected, accepted := scoredBuckets(heldOut.NotForMe.Scores), scoredBuckets(heldOut.Interesting.Scores)
	if len(rejected) != 10 || len(accepted) != 10 || slices.Max(rejected) >= slices.Min(accepted) {
		t.Fatalf("not for me %v, interesting %v", rejected, accepted)
	}
	if got, want := preview.Candidate.SuggestedThresholdTenths, suggestedThreshold(heldOut.Interesting); got != want {
		t.Fatalf("suggested threshold = %d, want %d", got, want)
	}
}

func TestPreviewKeepsOnlyCandidateAndActiveWeights(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	active := applyCandidate(t, s, 0)
	seedVerdicts(t, db, s, 11, 3, collectorapi.VerdictInteresting, "", "", "other:new")
	preview := previewScore(t, s, false)
	var weights int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_weights").Scan(&weights); err != nil {
		t.Fatal(err)
	}
	if want := active.Candidate.WeightCount + preview.Candidate.WeightCount; weights != want {
		t.Fatalf("stored weights = %d, want %d", weights, want)
	}
	for _, model := range storedScoreModels(t, db) {
		if !model.active && model.id != preview.Candidate.ID {
			t.Fatalf("stored model %+v", model)
		}
	}
}

func TestHeldOutTenthsCountScoresBeyondTheWindowAtItsEdge(t *testing.T) {
	const tagCount = 3000
	var evidence []evidenceGallery
	for id := int64(1); id <= 20; id++ {
		gallery := evidenceGallery{galleryID: id, class: scoreNotForMe}
		first := int64(1)
		if id > 10 {
			gallery.class, first = scoreInteresting, tagCount+1
		}
		for tagID := first; tagID < first+tagCount; tagID++ {
			gallery.tagIDs = append(gallery.tagIDs, tagID)
		}
		evidence = append(evidence, gallery)
	}
	inside := evidenceGallery{galleryID: 21, class: scoreNeutral, tagIDs: []int64{1}}
	evidence = append(evidence, inside)
	total, values := tallyScoreCounts(evidence)

	var train []evidenceGallery
	for _, gallery := range evidence {
		if gallery.galleryID%heldOutFolds != inside.galleryID%heldOutFolds {
			train = append(train, gallery)
		}
	}
	trainTotal, trainValues := tallyScoreCounts(train)
	weight, ok := scoreWeight(trainTotal, trainValues[scoreValue{kind: collectorapi.ScoreKindTag, tagID: 1}])
	if !ok {
		t.Fatal("tag 1 has no weight in the fold of gallery 21")
	}

	got := heldOutTenths(evidence, total, values, nil)
	for _, gallery := range evidence {
		want := -scoreWindowTenths
		switch {
		case gallery.galleryID == inside.galleryID:
			want = tenths(weight)
		case gallery.class == scoreInteresting:
			want = scoreWindowTenths
		}
		if got[gallery.galleryID] != want {
			t.Errorf("gallery %d: %d, want %d", gallery.galleryID, got[gallery.galleryID], want)
		}
	}
}
