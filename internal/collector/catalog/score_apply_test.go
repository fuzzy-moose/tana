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

// seedVerdicts retains count galleries from first on with the uploader, the
// category and the tags, each with the verdict.
func seedVerdicts(t *testing.T, db *pgxpool.Pool, s *Service, first int64, count int, verdict, uploader, category string, tags ...string) {
	t.Helper()
	for id := first; id < first+int64(count); id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Uploader: uploader, Category: category, Tags: tags}, true)
		setVerdict(t, s, id, verdict)
	}
}

var scoreArtists = []string{"artist:a", "artist:b", "artist:c", "artist:d", "artist:e", "artist:f"}

// seedScoreEvidence gives galleries 1 to 5 "Not for me" with female:glasses,
// 1 to 3 also with six artist tags and the uploader "Bad One", and galleries
// 6 to 10 "Interesting" with male:hat, 6 to 8 also in the category Manga. A
// model from it weighs female:glasses at about -0.69, each artist tag and the
// uploader at about -0.47, male:hat at about 0.69 and manga at about 0.47.
func seedScoreEvidence(t *testing.T, db *pgxpool.Pool, s *Service) {
	t.Helper()
	seedVerdicts(t, db, s, 1, 3, collectorapi.VerdictNotForMe, "Bad One", "", append([]string{"female:glasses"}, scoreArtists...)...)
	seedVerdicts(t, db, s, 4, 2, collectorapi.VerdictNotForMe, "", "", "female:glasses")
	seedVerdicts(t, db, s, 6, 3, collectorapi.VerdictInteresting, "", "Manga", "male:hat")
	seedVerdicts(t, db, s, 9, 2, collectorapi.VerdictInteresting, "", "", "male:hat")
}

// seedScoredGalleries retains galleries without a verdict: 101 with
// female:glasses (score tenths -7), 102 with male:hat (6), 103 without tags
// (0) and 104 with the artist tags and female:glasses (-36).
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

func applyCandidate(t *testing.T, s *Service, threshold int) collectorapi.ScorePreview {
	t.Helper()
	preview := previewScore(t, s, false)
	if preview.Candidate == nil {
		t.Fatalf("preview = %+v", preview)
	}
	if err := s.ApplyScoreModel(t.Context(), preview.Candidate.ID, threshold); err != nil {
		t.Fatal(err)
	}
	return preview
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

func weightOf(t *testing.T, preview collectorapi.ScorePreview, kind, label string) float64 {
	t.Helper()
	weight, ok := findWeight(preview, kind, label)
	if !ok {
		t.Fatalf("no weight for %s %s", kind, label)
	}
	return weight.Weight
}

func TestAppliedModelHidesGalleriesBelowTheThreshold(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
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
	preview := applyCandidate(t, s, 0)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleNeverHide)
	setVerdict(t, s, 106, collectorapi.VerdictInteresting)
	setVerdict(t, s, 107, collectorapi.VerdictNotForMe)

	on := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, on, []int64{109, 106, 105, 103, 102, 10, 9, 8, 7, 6})
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, hidden, []int64{108, 107, 104, 101, 5, 4, 3, 2, 1})
	all := scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, all, []int64{109, 108, 107, 106, 105, 104, 103, 102, 101, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1})
	for _, result := range []collectorapi.CatalogResult{on, hidden, all} {
		assertThreshold(t, result, 0)
	}
	for _, result := range []collectorapi.CatalogResult{hidden, all} {
		for _, id := range []int64{101, 104, 108} {
			if score := itemByID(t, result, id).Score; score == nil || score.Rule != collectorapi.ScoreRuleBelowThreshold || score.Override != "" {
				t.Fatalf("gallery %d score = %+v", id, score)
			}
		}
		if score := itemByID(t, result, 107).Score; score == nil || score.Rule != "" {
			t.Fatalf("gallery 107 score = %+v", score)
		}
	}
	if score := itemByID(t, on, 105).Score; score == nil || score.Rule != collectorapi.ScoreRuleNeverHide || score.Override != "Some One" {
		t.Fatalf("gallery 105 score = %+v", score)
	}

	glasses := weightOf(t, preview, collectorapi.ScoreKindTag, "female:glasses")
	artist := weightOf(t, preview, collectorapi.ScoreKindTag, "artist:a")
	hat := weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat")
	uploader := weightOf(t, preview, collectorapi.ScoreKindUploader, "Bad One")
	manga := weightOf(t, preview, collectorapi.ScoreKindCategory, "manga")
	for _, tc := range []struct {
		id      int64
		value   float64
		weights []collectorapi.CatalogScoreWeight
	}{
		{102, hat, []collectorapi.CatalogScoreWeight{{Kind: collectorapi.ScoreKindTag, Label: "male:hat", Weight: hat}}},
		{103, 0, nil},
		{108, uploader, []collectorapi.CatalogScoreWeight{{Kind: collectorapi.ScoreKindUploader, Label: "Bad One", Weight: uploader}}},
		{109, hat + manga, []collectorapi.CatalogScoreWeight{
			{Kind: collectorapi.ScoreKindTag, Label: "male:hat", Weight: hat},
			{Kind: collectorapi.ScoreKindCategory, Label: "manga", Weight: manga},
		}},
		{104, glasses + 6*artist, []collectorapi.CatalogScoreWeight{
			{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: glasses},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:a", Weight: artist},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:b", Weight: artist},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:c", Weight: artist},
			{Kind: collectorapi.ScoreKindTag, Label: "artist:d", Weight: artist},
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
	for _, tag := range itemByID(t, all, 104).SummaryTags {
		if tag.Namespace == "female" {
			t.Fatalf("summary tags hold the strongest weight: %+v", itemByID(t, all, 104).SummaryTags)
		}
	}
}

func TestApplyKeepsTheActiveModelForAMissingCandidate(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	first := applyCandidate(t, s, 0)
	active := activeModelRow(t, db)
	before := scorePage(t, s, "", collectorapi.CatalogOptions{})

	replaced := previewScore(t, s, false)
	unchanged := func() {
		t.Helper()
		if got := activeModelRow(t, db); got != active || got.id != first.Candidate.ID {
			t.Fatalf("active model = %+v, want %+v", got, active)
		}
		if after := scorePage(t, s, "", collectorapi.CatalogOptions{}); !reflect.DeepEqual(before, after) {
			t.Fatalf("listing changed:\n%+v\n%+v", before, after)
		}
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), replaced.Candidate.ID+100, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("unknown candidate: %v", err)
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), first.Candidate.ID, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("active model as candidate: %v", err)
	}
	unchanged()
	latest := previewScore(t, s, false)
	if err := s.ApplyScoreModel(t.Context(), replaced.Candidate.ID, 5); !errors.Is(err, ErrScoreCandidateNotFound) {
		t.Fatalf("replaced candidate: %v", err)
	}
	unchanged()
	if err := s.ApplyScoreModel(t.Context(), latest.Candidate.ID, -8); err != nil {
		t.Fatal(err)
	}
	models := storedScoreModels(t, db)
	if len(models) != 1 || !models[0].active || models[0].id != latest.Candidate.ID || models[0].threshold != -8 ||
		models[0].weights != latest.Candidate.WeightCount {
		t.Fatalf("models = %+v", models)
	}
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{104, 5, 4, 3, 2, 1})
}

func TestApplyWaitsForARunningPreview(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	applyCandidate(t, s, 0)
	active := activeModelRow(t, db)
	candidate := previewScore(t, s, false)

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
	go func() { errs <- s.ApplyScoreModel(t.Context(), candidate.Candidate.ID, 5) }()
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
	seedScoreEvidence(t, db, s)
	first := previewScore(t, s, false)
	var second int64
	if err := db.QueryRow(t.Context(), `INSERT INTO panda_score_models (favorites, calculated_at) VALUES (false, now())
		RETURNING id`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	blocker, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(t.Context())
	if _, err := blocker.Exec(t.Context(), "LOCK TABLE panda_score_models IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	for i, id := range []int64{first.Candidate.ID, second} {
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
	seedScoreEvidence(t, db, s)
	first := applyCandidate(t, s, 0)
	second := applyCandidate(t, s, 3)
	models := storedScoreModels(t, db)
	if len(models) != 1 || models[0].id != second.Candidate.ID || !models[0].active || models[0].threshold != 3 {
		t.Fatalf("models = %+v", models)
	}
	var left int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_weights WHERE model_id = $1", first.Candidate.ID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d weights of the first model remain", left)
	}
}

func TestSetScoreThresholdChangesWhatIsHidden(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	if err := s.SetScoreThreshold(t.Context(), 0); !errors.Is(err, ErrNoActiveScoreModel) {
		t.Fatalf("without a model: %v", err)
	}
	candidate := previewScore(t, s, false)
	for _, threshold := range []int{math.MinInt32 - 1, math.MaxInt32 + 1} {
		if err := s.ApplyScoreModel(t.Context(), candidate.Candidate.ID, threshold); !errors.Is(err, ErrInvalidScoreThreshold) {
			t.Fatalf("apply %d: %v", threshold, err)
		}
		if err := s.SetScoreThreshold(t.Context(), threshold); !errors.Is(err, ErrInvalidScoreThreshold) {
			t.Fatalf("set %d: %v", threshold, err)
		}
	}
	if models := storedScoreModels(t, db); len(models) != 1 || models[0].active {
		t.Fatalf("models = %+v", models)
	}
	if err := s.ApplyScoreModel(t.Context(), candidate.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	before := activeModelRow(t, db)
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{104, 101, 5, 4, 3, 2, 1})
	if err := s.SetScoreThreshold(t.Context(), -8); err != nil {
		t.Fatal(err)
	}
	result := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{103, 102, 101, 10, 9, 8, 7, 6})
	assertThreshold(t, result, -8)
	after := activeModelRow(t, db)
	if after.id != before.id || after.weights != before.weights || after.sum != before.sum || after.threshold != -8 {
		t.Fatalf("model before %+v, after %+v", before, after)
	}
	if err := s.SetScoreThreshold(t.Context(), 1001); err != nil {
		t.Fatal(err)
	}
	result = scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{10, 9, 8, 7, 6})
	assertThreshold(t, result, 1001)
	for _, threshold := range []int{math.MinInt32, math.MaxInt32} {
		if err := s.SetScoreThreshold(t.Context(), threshold); err != nil {
			t.Fatalf("set %d: %v", threshold, err)
		}
	}
}

func TestRemoveScoreModelKeepsOverrides(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	retain(t, db, panda.Metadata{ID: 105, Posted: 105, Uploader: "Some One"}, true)
	applyCandidate(t, s, 0)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleAlwaysHide)
	if err := s.RemoveScoreModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	result := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, result, []int64{104, 103, 102, 101, 10, 9, 8, 7, 6})
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
	seedScoreEvidence(t, db, s)
	applyCandidate(t, s, 0)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 201, Token: "token201"}, Title: "Feed",
		Tags: []string{"female:glasses"}}, time.Now().UTC())
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{Query: "Feed"}), []int64{201})
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{Query: "Feed"}), []int64{})
	retain(t, db, panda.Metadata{ID: 201, Posted: 201, Title: "Feed", Tags: []string{"male:hat"}}, true)
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{Query: "Feed"}), []int64{201})
}

func TestGalleryHiddenByItsScoreStaysUnseen(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	applyCandidate(t, s, 0)
	page := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	assertCatalogIDs(t, page, []int64{103, 102})
	if err := s.RecordDisplays(t.Context(), []int64{103, 102}, ""); err != nil {
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
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{101})
}

func TestVerdictViewIgnoresTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	views := map[string][]int64{
		collectorapi.VerdictInteresting: {10, 9, 8, 7, 6},
		collectorapi.VerdictNotForMe:    {5, 4, 3, 2, 1},
	}
	for verdict, want := range views {
		assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{Verdict: verdict}), want)
	}
	applyCandidate(t, s, 1000)
	for verdict, want := range views {
		result := scorePage(t, s, "", collectorapi.CatalogOptions{Verdict: verdict})
		assertCatalogIDs(t, result, want)
		assertThreshold(t, result, 1000)
		for _, item := range result.Items {
			if item.Score == nil || item.Score.Value == nil {
				t.Fatalf("gallery %d score = %+v", item.GalleryID, item.Score)
			}
		}
	}
}
