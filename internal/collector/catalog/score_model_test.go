package catalog

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func previewScore(t *testing.T, s *Service, favorites bool) collectorapi.ScorePreview {
	t.Helper()
	preview, err := s.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{Favorites: &favorites})
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func findWeight(preview collectorapi.ScorePreview, kind, label string) (collectorapi.ScoreWeight, bool) {
	if preview.Candidate == nil {
		return collectorapi.ScoreWeight{}, false
	}
	for _, weight := range preview.Candidate.Weights {
		if weight.Kind == kind && weight.Label == label {
			return weight, true
		}
	}
	return collectorapi.ScoreWeight{}, false
}

func scoreModelIDs(t *testing.T, db *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := db.Query(t.Context(), "SELECT id FROM panda_score_models ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}

func evidenceCounts(preview collectorapi.ScorePreview) [4]int {
	e := preview.Evidence
	return [4]int{e.Interesting, e.NotForMe, e.Neutral, e.CountedFavorites}
}

func TestPreviewCountsEachEvidenceGalleryOnce(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 4; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id}, true)
	}
	for range 3 {
		if err := s.RecordDisplays(t.Context(), []int64{1, 2, 99}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordDisplays(t.Context(), []int64{3}, collectorapi.ScoreFilterHidden); err != nil {
		t.Fatal(err)
	}
	setVerdict(t, s, 2, collectorapi.VerdictInteresting)
	setVerdict(t, s, 4, collectorapi.VerdictNotForMe)
	if _, err := db.Exec(t.Context(), `INSERT INTO panda_gallery_verdicts (gallery_id, verdict, given_at) VALUES (98, 'not_for_me', now())`); err != nil {
		t.Fatal(err)
	}
	preview := previewScore(t, s, true)
	if got := evidenceCounts(preview); got != [4]int{1, 1, 1, 0} {
		t.Fatalf("evidence = %v", got)
	}
	if preview.Evidence.FavoritesSyncedAt != nil || !preview.Evidence.FewVerdicts {
		t.Fatalf("evidence = %+v", preview.Evidence)
	}
}

func TestPreviewCountsFavoritesOnlyWithTheSwitch(t *testing.T) {
	db, s := openCatalog(t)
	tags := []string{"female:glasses"}
	for id := int64(1); id <= 3; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: tags}, true)
	}
	retain(t, db, panda.Metadata{ID: 4, Posted: 4}, true)
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES
		(1, 'https://panda.test', '42', 1, 'One', to_timestamp(200)), (2, 'https://panda.test', '43', 1, 'Two', to_timestamp(100)),
		(3, 'https://panda.test', '43', 2, 'Three', NULL);
		INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES
		(1, 1, 'token1', to_timestamp(100)), (2, 1, 'token1', to_timestamp(300)), (1, 2, 'token2', to_timestamp(200))`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDisplays(t.Context(), []int64{1}, ""); err != nil {
		t.Fatal(err)
	}
	setVerdict(t, s, 2, collectorapi.VerdictNotForMe)
	setVerdict(t, s, 3, collectorapi.VerdictInteresting)
	setVerdict(t, s, 4, collectorapi.VerdictNotForMe)

	with := previewScore(t, s, true)
	if got := evidenceCounts(with); got != [4]int{1, 2, 0, 1} || !with.Favorites {
		t.Fatalf("with favorites: evidence = %v, favorites %v", got, with.Favorites)
	}
	if weight, ok := findWeight(with, collectorapi.ScoreKindTag, "female:glasses"); !ok || weight.Interesting != 2 || weight.NotForMe != 1 || weight.Neutral != 0 {
		t.Fatalf("with favorites: weight = %+v, %v", weight, ok)
	}
	if at := with.Evidence.FavoritesSyncedAt; at == nil || !at.Equal(time.Unix(200, 0)) {
		t.Fatalf("favorites synced at = %v", at)
	}

	without := previewScore(t, s, false)
	if got := evidenceCounts(without); got != [4]int{1, 2, 1, 0} || without.Favorites || without.Candidate.Favorites {
		t.Fatalf("without favorites: evidence = %v, favorites %v", got, without.Favorites)
	}
	if weight, ok := findWeight(without, collectorapi.ScoreKindTag, "female:glasses"); !ok || weight.Interesting != 1 || weight.NotForMe != 1 || weight.Neutral != 1 {
		t.Fatalf("without favorites: weight = %+v, %v", weight, ok)
	}

	nilFavorites, err := s.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{})
	if err != nil || nilFavorites.Favorites || nilFavorites.Evidence.CountedFavorites != 0 {
		t.Fatalf("nil favorites = %+v, %v", nilFavorites, err)
	}
}

func TestPreviewWeighsTagsUploadersAndCategories(t *testing.T) {
	db, s := openCatalog(t)
	// Galleries 1 to 3 are "Not for me", 4 to 9 "Interesting", 10 neutral.
	for id := int64(1); id <= 3; id++ {
		tags := []string{"female:glasses"}
		if id != 2 {
			tags = append(tags, "other:rare")
		}
		retain(t, db, panda.Metadata{ID: id, Posted: id, Uploader: "Some One", Category: "Manga", Tags: tags}, true)
		setVerdict(t, s, id, collectorapi.VerdictNotForMe)
	}
	for id := int64(4); id <= 9; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: []string{"male:hat"}}, true)
		setVerdict(t, s, id, collectorapi.VerdictInteresting)
	}
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 10, Token: "token10"}, Title: "Feed",
		Tags: []string{"male:hat"}, Category: "doujinshi", Uploader: "Feed One"}, time.Now().UTC())
	setVerdict(t, s, 10, collectorapi.VerdictInteresting)
	retain(t, db, panda.Metadata{ID: 11, Posted: 11, Tags: []string{"female:glasses"}}, true)
	if err := s.RecordDisplays(t.Context(), []int64{11}, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetScoreOverride(t.Context(), collectorapi.ScoreKindTag, "female:Glasses", collectorapi.ScoreRuleAlwaysHide); err != nil {
		t.Fatal(err)
	}
	before, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}

	preview := previewScore(t, s, false)
	if preview.Candidate == nil || preview.NoModel != "" {
		t.Fatalf("preview = %+v", preview)
	}
	glasses, ok := findWeight(preview, collectorapi.ScoreKindTag, "female:glasses")
	if !ok || glasses.Weight >= 0 || glasses.Interesting != 0 || glasses.NotForMe != 3 || glasses.Neutral != 1 {
		t.Fatalf("glasses = %+v, %v", glasses, ok)
	}
	if len(preview.Overrides) != 1 || preview.Overrides[0].Label != glasses.Label || preview.Overrides[0].Kind != collectorapi.ScoreKindTag ||
		preview.Overrides[0].Rule != collectorapi.ScoreRuleAlwaysHide || preview.Overrides[0].ID <= 0 {
		t.Fatalf("overrides = %+v", preview.Overrides)
	}
	if hat, ok := findWeight(preview, collectorapi.ScoreKindTag, "male:hat"); !ok || hat.Weight <= 0 || hat.Interesting != 7 {
		t.Fatalf("hat (with feed tags) = %+v, %v", hat, ok)
	}
	if uploader, ok := findWeight(preview, collectorapi.ScoreKindUploader, "Some One"); !ok || uploader.Weight >= 0 {
		t.Fatalf("uploader = %+v, %v", uploader, ok)
	}
	if category, ok := findWeight(preview, collectorapi.ScoreKindCategory, "manga"); !ok || category.Weight >= 0 {
		t.Fatalf("category = %+v, %v", category, ok)
	}
	if _, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:rare"); ok {
		t.Fatal("tag on two galleries has a weight")
	}
	weights := preview.Candidate.Weights
	for i, weight := range weights {
		if weight.Label == "" {
			t.Fatalf("weight without label: %+v", weight)
		}
		if i > 0 && math.Abs(weight.Weight) > math.Abs(weights[i-1].Weight) {
			t.Fatalf("weights out of order: %+v", weights)
		}
	}
	var stored int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_weights WHERE model_id = $1", preview.Candidate.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != preview.Candidate.WeightCount || stored != len(weights) {
		t.Fatalf("stored %d, weight count %d, returned %d", stored, preview.Candidate.WeightCount, len(weights))
	}
	if preview.Candidate.CalculatedAt.IsZero() || preview.Candidate.CalculatedAt.Location() != time.UTC {
		t.Fatalf("calculated at = %v", preview.Candidate.CalculatedAt)
	}

	second := previewScore(t, s, false)
	if ids := scoreModelIDs(t, db); len(ids) != 1 || ids[0] != second.Candidate.ID || second.Candidate.ID == preview.Candidate.ID {
		t.Fatalf("models = %v, second %d", ids, second.Candidate.ID)
	}
	after, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("listing changed:\n%+v\n%+v", before, after)
	}
}

func TestPreviewWithoutAModelNamesTheMissingSide(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 4; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id}, true)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_categories (id, host, account_key, category, name) VALUES (1, 'https://panda.test', '42', 1, 'One');
		INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES (1, 1, 'token1', now())`); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordDisplays(t.Context(), []int64{2}, ""); err != nil {
		t.Fatal(err)
	}
	// A stored candidate from earlier evidence does not survive a preview without a model.
	if _, err := db.Exec(t.Context(), "INSERT INTO panda_score_models (favorites, calculated_at) VALUES (false, now())"); err != nil {
		t.Fatal(err)
	}
	expect := func(want string) {
		t.Helper()
		preview := previewScore(t, s, true)
		if preview.Candidate != nil || preview.NoModel != want {
			t.Fatalf("preview = %+v, want %q", preview, want)
		}
		if ids := scoreModelIDs(t, db); len(ids) != 0 {
			t.Fatalf("models = %v", ids)
		}
	}
	expect(collectorapi.ScoreNoVerdicts)
	setVerdict(t, s, 3, collectorapi.VerdictInteresting)
	expect(collectorapi.ScoreNoNotForMe)

	db2, s2 := openCatalog(t)
	retain(t, db2, panda.Metadata{ID: 1, Posted: 1}, true)
	setVerdict(t, s2, 1, collectorapi.VerdictNotForMe)
	if preview := previewScore(t, s2, false); preview.Candidate != nil || preview.NoModel != collectorapi.ScoreNoAcceptable {
		t.Fatalf("preview = %+v", preview)
	}
	if ids := scoreModelIDs(t, db2); len(ids) != 0 {
		t.Fatalf("models = %v", ids)
	}
}

func TestPreviewJSONHasEmptyOverridesAndFewVerdicts(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 2*FewVerdicts; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id}, true)
		verdict := collectorapi.VerdictInteresting
		if id%2 == 0 {
			verdict = collectorapi.VerdictNotForMe
		}
		setVerdict(t, s, id, verdict)
	}
	preview := previewScore(t, s, false)
	if preview.Evidence.FewVerdicts {
		t.Fatalf("evidence = %+v", preview.Evidence)
	}
	setVerdict(t, s, 1, collectorapi.VerdictNotForMe)
	preview = previewScore(t, s, false)
	if !preview.Evidence.FewVerdicts {
		t.Fatalf("evidence = %+v", preview.Evidence)
	}
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`"overrides":[]`, `"weights":[]`, `"favorites_synced_at":null`} {
		if !strings.Contains(string(body), part) {
			t.Fatalf("preview JSON lacks %s: %s", part, body)
		}
	}
	if strings.Contains(string(body), "no_model") {
		t.Fatalf("preview JSON with a candidate has no_model: %s", body)
	}
}

func TestPreviewRejectsAnInvalidDefaultFilter(t *testing.T) {
	_, s := openCatalog(t)
	for _, tc := range []struct {
		options collectorapi.ScorePreviewOptions
		want    error
	}{
		{collectorapi.ScorePreviewOptions{DefaultQuery: `"unfinished`}, ErrInvalidQuery},
		{collectorapi.ScorePreviewOptions{DefaultCategories: []string{"unknown"}}, panda.ErrInvalidCategory},
	} {
		if _, err := s.PreviewScoreModel(t.Context(), tc.options); !errors.Is(err, tc.want) {
			t.Fatalf("%+v: %v, want %v", tc.options, err, tc.want)
		}
	}
}
