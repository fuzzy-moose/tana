package catalog

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// seedScoredGalleries retains galleries 101 with female:glasses (score tenths
// -7 under storedScoreWeights), 102 with male:hat (6), 103 without tags (0)
// and 104 with the artist tags and female:glasses (-36).
func seedScoredGalleries(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	for _, entry := range []panda.Metadata{
		{ID: 101, Posted: 101, Tags: []string{"female:glasses"}},
		{ID: 102, Posted: 102, Tags: []string{"male:hat"}},
		{ID: 103, Posted: 103},
		{ID: 104, Posted: 104, Tags: append([]string{"female:glasses"}, scoreArtists...)},
	} {
		retain(t, db, entry, true)
	}
}

type storedScoreModel struct {
	id        int64
	active    bool
	threshold int
	weights   int
	sum       float64
}

func storedScoreModels(t *testing.T, db *pgxpool.Pool) []storedScoreModel {
	t.Helper()
	rows, err := db.Query(t.Context(), `SELECT m.id, m.active, COALESCE(m.threshold_tenths, 0), count(w.model_id), COALESCE(sum(w.weight), 0)
		FROM panda_score_models m LEFT JOIN panda_score_weights w ON w.model_id = m.id GROUP BY m.id ORDER BY m.id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var models []storedScoreModel
	for rows.Next() {
		var model storedScoreModel
		if err := rows.Scan(&model.id, &model.active, &model.threshold, &model.weights, &model.sum); err != nil {
			t.Fatal(err)
		}
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return models
}

func activeModelRow(t *testing.T, db *pgxpool.Pool) storedScoreModel {
	t.Helper()
	var found []storedScoreModel
	for _, model := range storedScoreModels(t, db) {
		if model.active {
			found = append(found, model)
		}
	}
	if len(found) != 1 {
		t.Fatalf("active models = %+v", found)
	}
	return found[0]
}

func assertThreshold(t *testing.T, result collectorapi.CatalogResult, want int) {
	t.Helper()
	if result.ScoreFilter == nil || result.ScoreFilter.ThresholdTenths == nil || *result.ScoreFilter.ThresholdTenths != want {
		t.Fatalf("score filter = %+v, want threshold %d", result.ScoreFilter, want)
	}
}

func TestAppliedModelHidesGalleriesBelowTheThreshold(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	for _, entry := range []panda.Metadata{
		{ID: 105, Posted: 105, Tags: []string{"female:glasses"}, Uploader: "Some One"},
		{ID: 106, Posted: 106, Tags: []string{"female:glasses"}},
		{ID: 107, Posted: 107, Tags: []string{"male:hat"}},
		{ID: 108, Posted: 108, Uploader: "Bad One"},
		{ID: 109, Posted: 109, Category: "Manga", Tags: []string{"male:hat"}},
	} {
		retain(t, db, entry, true)
	}
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleNeverHide)

	on := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, on, []int64{109, 107, 105, 103, 102, 10, 9, 8, 7, 6})
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, hidden, []int64{108, 106, 104, 101, 5, 4, 3, 2, 1})
	all := scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, all, []int64{109, 108, 107, 106, 105, 104, 103, 102, 101, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
	for _, result := range []collectorapi.CatalogResult{on, hidden, all} {
		assertThreshold(t, result, 0)
	}
	for _, result := range []collectorapi.CatalogResult{hidden, all} {
		for _, id := range []int64{101, 104, 106, 108} {
			if score := itemByID(t, result, id).Score; score == nil || score.Rule != collectorapi.ScoreRuleBelowThreshold || score.Override != "" {
				t.Fatalf("gallery %d score = %+v", id, score)
			}
		}
	}
	for _, result := range []collectorapi.CatalogResult{on, all} {
		if score := itemByID(t, result, 107).Score; score == nil || score.Rule != "" {
			t.Fatalf("gallery 107 score = %+v", score)
		}
	}
	if score := itemByID(t, on, 105).Score; score == nil || score.Rule != collectorapi.ScoreRuleNeverHide || score.Override != "Some One" {
		t.Fatalf("gallery 105 score = %+v", score)
	}

	for _, tc := range []struct {
		id      int64
		value   float64
		weights []collectorapi.CatalogScoreWeight
	}{
		{102, hatWeight, []collectorapi.CatalogScoreWeight{{Kind: collectorapi.ScoreKindTag, Label: "male:hat", Weight: hatWeight}}},
		{103, 0, nil},
		{108, uploaderWeight, []collectorapi.CatalogScoreWeight{{Kind: collectorapi.ScoreKindUploader, Label: "Bad One", Weight: uploaderWeight}}},
		{109, hatWeight + mangaWeight, []collectorapi.CatalogScoreWeight{
			{Kind: collectorapi.ScoreKindTag, Label: "male:hat", Weight: hatWeight},
			{Kind: collectorapi.ScoreKindCategory, Label: "manga", Weight: mangaWeight},
		}},
		{104, glassesWeight + 6*artistWeight, []collectorapi.CatalogScoreWeight{
			{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: glassesWeight},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:a", Weight: artistWeight},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:b", Weight: artistWeight},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:c", Weight: artistWeight},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:d", Weight: artistWeight},
		}},
	} {
		item := itemByID(t, all, tc.id)
		if item.Score == nil || item.Score.Value == nil || math.Abs(*item.Score.Value-tc.value) > 1e-9 {
			t.Fatalf("gallery %d score = %+v, want value %v", tc.id, item.Score, tc.value)
		}
		if len(item.Score.Weights) != len(tc.weights) {
			t.Fatalf("gallery %d weights = %+v, want %+v", tc.id, item.Score.Weights, tc.weights)
		}
		for i, weight := range item.Score.Weights {
			want := tc.weights[i]
			if weight.Kind != want.Kind || weight.Label != want.Label || math.Abs(weight.Weight-want.Weight) > 1e-9 {
				t.Fatalf("gallery %d weights = %+v, want %+v", tc.id, item.Score.Weights, tc.weights)
			}
		}
	}
	ranked := []summaryTag{weighted("female:glasses", glassesWeight)}
	for _, label := range scoreArtists[:5] {
		ranked = append(ranked, weighted(label, artistWeight))
	}
	assertSummaryTags(t, itemByID(t, all, 104), ranked)
}

func TestApplyKeepsTheActiveModelForAMissingCandidate(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	first := applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	active := activeModelRow(t, db)
	before := scorePage(t, s, "", collectorapi.CatalogOptions{})

	replaced := storeScoreCandidate(t, db, storedScoreWeights()...)
	unchanged := func() {
		t.Helper()
		if got := activeModelRow(t, db); got != active || got.id != first {
			t.Fatalf("active model = %+v, want %+v", got, active)
		}
		if after := scorePage(t, s, "", collectorapi.CatalogOptions{}); !reflect.DeepEqual(before, after) {
			t.Fatalf("listing changed:\n%+v\n%+v", before, after)
		}
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), replaced+100, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("unknown candidate: %v", err)
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), first, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("active model as candidate: %v", err)
	}
	unchanged()
	// A preview without evidence deletes every candidate and stores none.
	if preview := previewScore(t, s); preview.Candidate != nil {
		t.Fatalf("preview = %+v", preview)
	}
	latest := storeScoreCandidate(t, db, storedScoreWeights()...)
	if err := s.ApplyScoreModel(t.Context(), replaced, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("replaced candidate: %v", err)
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), latest, -8); err != nil {
		t.Fatal(err)
	}
	models := storedScoreModels(t, db)
	if len(models) != 1 || !models[0].active || models[0].id != latest || models[0].threshold != -8 ||
		models[0].weights != len(storedScoreWeights()) {
		t.Fatalf("models = %+v", models)
	}
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{104, 3, 2, 1})
}

func TestApplyWaitsForARunningPreview(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	active := activeModelRow(t, db)
	candidate := storeScoreCandidate(t, db, storedScoreWeights()...)

	// A preview locks the table and deletes the candidate before it stores its own.
	blocker, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(t.Context())
	if _, err := blocker.Exec(t.Context(), "LOCK TABLE panda_score_models IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	if _, err := blocker.Exec(t.Context(), "DELETE FROM panda_score_models WHERE NOT active"); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 1)
	go func() { errs <- s.ApplyScoreModel(t.Context(), candidate, 5) }()
	waitForLockWaiters(t, db, 1)
	select {
	case err := <-errs:
		t.Fatalf("apply returned during the preview: %v", err)
	default:
	}
	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := <-errs; !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("apply after the preview: %v", err)
	}
	if got := activeModelRow(t, db); got != active {
		t.Fatalf("active model = %+v, want %+v", got, active)
	}
}

func TestConcurrentAppliesLeaveOneActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	first := storeScoreCandidate(t, db, storedScoreWeights()...)
	second := storeScoreCandidate(t, db)
	blocker, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(t.Context())
	if _, err := blocker.Exec(t.Context(), "LOCK TABLE panda_score_models IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	for i, id := range []int64{first, second} {
		go func() { errs <- s.ApplyScoreModel(t.Context(), id, i) }()
		waitForLockWaiters(t, db, i+1)
	}
	if err := blocker.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if models := storedScoreModels(t, db); len(models) != 1 || !models[0].active {
		t.Fatalf("models = %+v", models)
	}
}

func TestApplyingASecondCandidateDeletesTheFirst(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	first := applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	second := applyScoreModel(t, db, s, 3, storedScoreWeights()[:2]...)
	models := storedScoreModels(t, db)
	if len(models) != 1 || models[0].id != second || !models[0].active || models[0].threshold != 3 || models[0].weights != 2 {
		t.Fatalf("models = %+v", models)
	}
	var left int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_weights WHERE model_id = $1", first).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d weights of the first model remain", left)
	}
}

func TestSetScoreThresholdChangesWhatIsHidden(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	if err := s.SetScoreThreshold(t.Context(), 0); !errors.Is(err, ErrNoActiveScoreModel) {
		t.Fatalf("without a model: %v", err)
	}
	candidate := storeScoreCandidate(t, db, storedScoreWeights()...)
	for _, threshold := range []int{math.MinInt32 - 1, math.MaxInt32 + 1} {
		if err := s.ApplyScoreModel(t.Context(), candidate, threshold); !errors.Is(err, ErrInvalidScoreThreshold) {
			t.Fatalf("apply %d: %v", threshold, err)
		}
		if err := s.SetScoreThreshold(t.Context(), threshold); !errors.Is(err, ErrInvalidScoreThreshold) {
			t.Fatalf("set %d: %v", threshold, err)
		}
	}
	if models := storedScoreModels(t, db); len(models) != 1 || models[0].active {
		t.Fatalf("models = %+v", models)
	}
	if err := s.ApplyScoreModel(t.Context(), candidate, 0); err != nil {
		t.Fatal(err)
	}
	before := activeModelRow(t, db)
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{104, 101, 5, 4, 3, 2, 1})
	if err := s.SetScoreThreshold(t.Context(), -8); err != nil {
		t.Fatal(err)
	}
	result := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{103, 102, 101, 10, 9, 8, 7, 6, 5, 4})
	assertThreshold(t, result, -8)
	after := activeModelRow(t, db)
	if after.id != before.id || after.weights != before.weights || after.sum != before.sum || after.threshold != -8 {
		t.Fatalf("model before %+v, after %+v", before, after)
	}
	if err := s.SetScoreThreshold(t.Context(), 11); err != nil {
		t.Fatal(err)
	}
	result = scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{8, 7, 6})
	assertThreshold(t, result, 11)
	for _, threshold := range []int{math.MinInt32, math.MaxInt32} {
		if err := s.SetScoreThreshold(t.Context(), threshold); err != nil {
			t.Fatalf("set %d: %v", threshold, err)
		}
	}
}

func TestRemoveScoreModelKeepsOverrides(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	retain(t, db, panda.Metadata{ID: 105, Posted: 105, Uploader: "Some One"}, true)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleAlwaysHide)
	if err := s.RemoveScoreModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	result := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{104, 103, 102, 101, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
	if result.ScoreFilter == nil || result.ScoreFilter.ThresholdTenths != nil {
		t.Fatalf("score filter = %+v", result.ScoreFilter)
	}
	for _, item := range result.Items {
		assertScore(t, item, "", "")
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM panda_score_overrides"); err != nil {
		t.Fatal(err)
	}
	result = scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{105, 104, 103, 102, 101, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
	if result.ScoreFilter != nil {
		t.Fatalf("score filter = %+v", result.ScoreFilter)
	}
	if err := s.RemoveScoreModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	if models := storedScoreModels(t, db); len(models) != 0 {
		t.Fatalf("models = %+v", models)
	}
}

func TestAppliedModelScoresProvisionalThenCollectedTags(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 201, Token: "token201"}, Title: "Feed",
		Tags: []string{"female:glasses"}}, time.Now().UTC())
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{Query: "Feed"}), []int64{201})
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{Query: "Feed"}), []int64{})
	retain(t, db, panda.Metadata{ID: 201, Posted: 201, Title: "Feed", Tags: []string{"male:hat"}}, true)
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{Query: "Feed"}), []int64{201})
}

func TestGalleryHiddenByItsScoreStaysUnseen(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	page := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	shown := []int64{103, 102, 10, 9, 8, 7, 6}
	assertCatalogIDs(t, page, shown)
	if err := s.RecordDisplays(t.Context(), shown); err != nil {
		t.Fatal(err)
	}
	empty := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	assertCatalogIDs(t, empty, []int64{})
	if empty.ScoreFilter == nil || !empty.ScoreFilter.HiddenUnseen {
		t.Fatalf("score filter = %+v", empty.ScoreFilter)
	}
	if err := s.SetScoreThreshold(t.Context(), -8); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{101, 5, 4})
}
