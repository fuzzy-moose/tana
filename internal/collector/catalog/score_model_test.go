package catalog

import (
	"encoding/json"
	"errors"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func previewScore(t *testing.T, s *Service) collectorapi.ScorePreview {
	t.Helper()
	preview, err := s.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{})
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

// favor makes the galleries favorites of the favorite collection category of
// the account, creating the collection and the gallery refs when missing.
func favor(t *testing.T, db *pgxpool.Pool, account string, category int, ids ...int64) {
	t.Helper()
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_categories (host, account_key, category, name)
		VALUES ('https://panda.test', $1, $2, '') ON CONFLICT DO NOTHING;`, account, category); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token)
		SELECT id, 'token' || id FROM unnest($1::bigint[]) id ON CONFLICT DO NOTHING`, ids); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO favorites (category_id, gallery_id, token, added_at)
		SELECT c.id, g, 'token' || g, now() FROM favorite_categories c, unnest($3::bigint[]) g
		WHERE c.account_key = $1 AND c.category = $2`, account, category, ids); err != nil {
		t.Fatal(err)
	}
}

// retainRange retains the galleries from first to last with the entry's
// values.
func retainRange(t *testing.T, db *pgxpool.Pool, first, last int64, entry panda.Metadata) {
	t.Helper()
	for id := first; id <= last; id++ {
		entry.ID, entry.Posted = id, id
		retain(t, db, entry, true)
	}
}

func TestPreviewCountsEachFavoriteInScopeOnce(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Category: "Manga"})
	retain(t, db, panda.Metadata{ID: 5, Posted: 5, Category: "Doujinshi"}, true)
	retain(t, db, panda.Metadata{ID: 6, Posted: 6, Category: "Manga", Tags: []string{"language:english"}}, true)
	retain(t, db, panda.Metadata{ID: 7, Posted: 7, Category: "Manga", Expunged: true}, true)
	retain(t, db, panda.Metadata{ID: 8, Posted: 8, Category: "Manga"}, true)
	// Gallery 3 is a favorite of both accounts; gallery 4 has no catalog
	// entry; gallery 8 is no favorite.
	favor(t, db, "42", 0, 1, 3, 4, 5, 6, 7)
	favor(t, db, "43", 1, 2, 3)
	options := collectorapi.ScorePreviewOptions{DefaultQuery: "-l:english$", DefaultCategories: []string{"manga"}}
	if got := previewWith(t, s, options).Evidence.Favorites; got != 4 {
		t.Fatalf("favorites in scope = %d, want 4", got)
	}
	if got := previewScore(t, s).Evidence.Favorites; got != 6 {
		t.Fatalf("favorites without a default filter = %d, want 6", got)
	}
}

func TestPreviewWeighsValuesByTheirLiftAgainstTheCatalog(t *testing.T) {
	db, s := openCatalog(t)
	// Favorites 1 to 4 in the catalog of 20: other:a on 3 favorites and 5
	// galleries, other:common on 3 and 15, other:pair on 2 and 2, the
	// uploader "Up" on 3 and 3, the category manga on 4 and 6.
	retainRange(t, db, 1, 3, panda.Metadata{Uploader: "Up", Category: "Manga", Tags: []string{"other:a", "other:common", "other:pair"}})
	retain(t, db, panda.Metadata{ID: 4, Posted: 4, Category: "Manga"}, true)
	retainRange(t, db, 5, 6, panda.Metadata{Category: "Manga", Tags: []string{"other:a", "other:common"}})
	retainRange(t, db, 7, 16, panda.Metadata{Tags: []string{"other:common"}})
	retainRange(t, db, 17, 20, panda.Metadata{})
	if _, err := db.Exec(t.Context(), `DELETE FROM catalog_gallery_tags gt USING catalog_tags t
		WHERE t.id = gt.tag_id AND t.value = 'pair' AND gt.gallery_id = 3`); err != nil {
		t.Fatal(err)
	}
	favor(t, db, "42", 0, 1, 2, 3, 4)

	preview := previewScore(t, s)
	if preview.NoModel != "" || preview.Candidate == nil {
		t.Fatalf("no model %q, candidate %+v", preview.NoModel, preview.Candidate)
	}
	want := []collectorapi.ScoreWeight{
		{Kind: collectorapi.ScoreKindCategory, Label: "manga", Weight: math.Log(14 / 11.2), Favorites: 4, Catalog: 6},
		{Kind: collectorapi.ScoreKindUploader, Label: "Up", Weight: math.Log(13 / 10.6), Favorites: 3, Catalog: 3},
		{Kind: collectorapi.ScoreKindTag, Label: "other:a", Weight: math.Log(13.0 / 11), Favorites: 3, Catalog: 5},
	}
	got := preview.Candidate.Weights
	if len(got) != len(want) || preview.Candidate.WeightCount != len(want) {
		t.Fatalf("weights = %+v, count %d, want %+v", got, preview.Candidate.WeightCount, want)
	}
	for i := range want {
		if math.Abs(got[i].Weight-want[i].Weight) > 1e-12 {
			t.Fatalf("weight %d = %+v, want %+v", i, got[i], want[i])
		}
		got[i].Weight = want[i].Weight
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("weights = %+v, want %+v", got, want)
	}

	rows, err := db.Query(t.Context(), `SELECT w.kind, COALESCE(t.namespace || ':' || t.value, w.value), w.favorites, w.catalog, w.weight > 0
		FROM panda_score_weights w LEFT JOIN catalog_tags t ON t.id = w.tag_id WHERE w.model_id = $1`, preview.Candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	type storedCounts struct {
		favorites, catalog int
		positive           bool
	}
	stored := map[string]storedCounts{}
	for rows.Next() {
		var kind, label string
		var counts storedCounts
		if err := rows.Scan(&kind, &label, &counts.favorites, &counts.catalog, &counts.positive); err != nil {
			t.Fatal(err)
		}
		stored[kind+" "+label] = counts
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantStored := map[string]storedCounts{}
	for _, weight := range want {
		wantStored[weight.Kind+" "+weight.Label] = storedCounts{weight.Favorites, weight.Catalog, true}
	}
	if !reflect.DeepEqual(stored, wantStored) {
		t.Fatalf("stored weights = %v, want %v", stored, wantStored)
	}
	var favorites, catalog int
	if err := db.QueryRow(t.Context(), "SELECT favorites_in_scope, catalog_in_scope FROM panda_score_models WHERE id = $1",
		preview.Candidate.ID).Scan(&favorites, &catalog); err != nil {
		t.Fatal(err)
	}
	if favorites != 4 || catalog != 20 {
		t.Fatalf("stored model scope = %d favorites, %d catalog", favorites, catalog)
	}
}

func TestPreviewGivesAnUnknownUploaderOrCategoryNoWeight(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{})
	retainRange(t, db, 4, 10, panda.Metadata{Uploader: "Someone", Category: "Manga"})
	favor(t, db, "42", 0, 1, 2, 3)
	if candidate := previewScore(t, s).Candidate; candidate == nil || candidate.WeightCount != 0 {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestPreviewCountsTheCatalogInScope(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Category: "Manga", Tags: []string{"other:a"}})
	retain(t, db, panda.Metadata{ID: 4, Posted: 4, Category: "Manga", Expunged: true, Tags: []string{"other:a"}}, true)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 5, Token: "token5"}, Title: "Feed", Category: "manga", Tags: []string{"other:a"}}, time.Unix(5, 0))
	retainRange(t, db, 6, 10, panda.Metadata{Category: "Manga"})
	retainRange(t, db, 11, 20, panda.Metadata{Category: "Doujinshi", Tags: []string{"other:a"}})
	favor(t, db, "42", 0, 1, 2, 3)

	for _, tc := range []struct {
		name       string
		categories []string
		catalog    int
		weight     float64
	}{
		{"whole catalog", nil, 15, math.Log(13 / (10 + 3*15/20.0))},
		{"manga", []string{"manga"}, 5, math.Log(13 / (10 + 3*5/10.0))},
	} {
		preview := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: tc.categories})
		weight, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:a")
		if !ok || weight.Catalog != tc.catalog || math.Abs(weight.Weight-tc.weight) > 1e-12 {
			t.Fatalf("%s: other:a = %+v, %v; want catalog %d, weight %g", tc.name, weight, ok, tc.catalog, tc.weight)
		}
	}
}

func TestPreviewWithoutAWeightStillStoresACandidate(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 2, panda.Metadata{Tags: []string{"other:a"}})
	favor(t, db, "42", 0, 1)
	preview := previewScore(t, s)
	if preview.NoModel != "" || preview.Candidate == nil || preview.Candidate.WeightCount != 0 {
		t.Fatalf("no model %q, candidate %+v", preview.NoModel, preview.Candidate)
	}
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"weights":[]`) || strings.Contains(string(body), "no_model") {
		t.Fatalf("preview JSON = %s", body)
	}
	if got := scoreModelIDs(t, db); !reflect.DeepEqual(got, []int64{preview.Candidate.ID}) {
		t.Fatalf("models = %v", got)
	}

	if _, err := db.Exec(t.Context(), "DELETE FROM favorites"); err != nil {
		t.Fatal(err)
	}
	preview = previewScore(t, s)
	if preview.NoModel != collectorapi.ScoreNoFavorites || preview.Candidate != nil {
		t.Fatalf("no model %q, candidate %+v", preview.NoModel, preview.Candidate)
	}
	if got := scoreModelIDs(t, db); len(got) != 0 {
		t.Fatalf("models = %v", got)
	}
}

func TestPreviewEvidenceWarnsAndNamesTheLastSync(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, FewFavorites, panda.Metadata{})
	ids := make([]int64, 0, FewFavorites)
	for id := int64(1); id <= FewFavorites; id++ {
		ids = append(ids, id)
	}
	favor(t, db, "42", 0, ids[:FewFavorites-1]...)
	favor(t, db, "42", 1)
	evidence := previewScore(t, s).Evidence
	if evidence.Favorites != FewFavorites-1 || !evidence.FewFavorites || evidence.FavoritesSyncedAt != nil {
		t.Fatalf("evidence = %+v", evidence)
	}

	favor(t, db, "42", 1, ids[FewFavorites-1])
	synced := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if _, err := db.Exec(t.Context(), `UPDATE favorite_categories SET synced_at = CASE category WHEN 0 THEN $1::timestamptz ELSE $1::timestamptz - interval '1 day' END`,
		synced); err != nil {
		t.Fatal(err)
	}
	evidence = previewScore(t, s).Evidence
	if evidence.Favorites != FewFavorites || evidence.FewFavorites || evidence.FavoritesSyncedAt == nil || !evidence.FavoritesSyncedAt.Equal(synced) {
		t.Fatalf("evidence = %+v", evidence)
	}
}

// storedModelRows lists the stored weights of the model with their counts,
// and its threshold.
func storedModelRows(t *testing.T, db *pgxpool.Pool, id int64) []string {
	t.Helper()
	rows, err := db.Query(t.Context(), `SELECT concat_ws(' ', w.kind, w.tag_id, w.value, w.weight, w.favorites, w.catalog, m.threshold_tenths)
		FROM panda_score_weights w JOIN panda_score_models m ON m.id = w.model_id WHERE w.model_id = $1 ORDER BY 1`, id)
	if err != nil {
		t.Fatal(err)
	}
	var stored []string
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestPreviewLeavesTheActiveModelUnchanged(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Category: "Manga", Tags: []string{"other:a"}})
	retainRange(t, db, 4, 10, panda.Metadata{Category: "Doujinshi"})
	favor(t, db, "42", 0, 1, 2, 3)
	candidate := previewScore(t, s).Candidate
	if candidate == nil || candidate.WeightCount == 0 {
		t.Fatalf("candidate = %+v", candidate)
	}
	if err := s.ApplyScoreModel(t.Context(), candidate.ID, 4); err != nil {
		t.Fatal(err)
	}
	before := storedModelRows(t, db, candidate.ID)
	if len(before) != candidate.WeightCount {
		t.Fatalf("stored rows = %v", before)
	}
	previewScore(t, s)
	favor(t, db, "42", 0, 4, 5, 6)
	previewScore(t, s)
	previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: []string{"doujinshi"}})
	if after := storedModelRows(t, db, candidate.ID); !reflect.DeepEqual(after, before) {
		t.Fatalf("active model rows = %v, want %v", after, before)
	}
}

func TestPreviewKeepsOnlyCandidateAndActiveWeights(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Tags: []string{"other:a"}})
	retainRange(t, db, 4, 6, panda.Metadata{Tags: []string{"other:new"}})
	retainRange(t, db, 7, 10, panda.Metadata{})
	favor(t, db, "42", 0, 1, 2, 3)
	active := previewScore(t, s)
	if err := s.ApplyScoreModel(t.Context(), active.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	favor(t, db, "42", 0, 4, 5, 6)
	previewScore(t, s)
	preview := previewScore(t, s)
	if active.Candidate.WeightCount == 0 || preview.Candidate.WeightCount < 2 {
		t.Fatalf("active %+v, candidate %+v", active.Candidate, preview.Candidate)
	}
	var weights int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_weights").Scan(&weights); err != nil {
		t.Fatal(err)
	}
	if want := active.Candidate.WeightCount + preview.Candidate.WeightCount; weights != want {
		t.Fatalf("stored weights = %d, want %d", weights, want)
	}
	if got, want := scoreModelIDs(t, db), []int64{active.Candidate.ID, preview.Candidate.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}

func TestPreviewRecentCountsTheGalleriesTheListingShows(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Category: "Manga", Tags: []string{"other:a"}})
	retainRange(t, db, 4, 8, panda.Metadata{Category: "Manga"})
	retain(t, db, panda.Metadata{ID: 9, Posted: 9, Category: "Manga", Uploader: "Keep"}, true)
	retain(t, db, panda.Metadata{ID: 10, Posted: 10, Category: "Manga", Uploader: "Drop"}, true)
	retain(t, db, panda.Metadata{ID: 11, Posted: 11, Category: "Manga", Expunged: true}, true)
	retain(t, db, panda.Metadata{ID: 12, Posted: 12, Category: "Doujinshi"}, true)
	favor(t, db, "42", 0, 1, 2, 3)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Keep", collectorapi.ScoreRuleNeverHide)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Drop", collectorapi.ScoreRuleAlwaysHide)

	recent := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: []string{"manga"}}).Recent
	if recent.Candidate == nil || recent.Active != nil {
		t.Fatalf("recent = %+v", recent)
	}
	scored := 0
	for _, count := range recent.Candidate.Counts {
		scored += count
	}
	if recent.Total != 10 || recent.NeverHidden != 1 || recent.AlwaysHidden != 1 || recent.Total != recent.NeverHidden+recent.AlwaysHidden+scored {
		t.Fatalf("recent = %+v, %d scored", recent, scored)
	}
}

// jsonKeys returns the sorted keys of the JSON object.
func jsonKeys(t *testing.T, body []byte) []string {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatalf("%s: %v", body, err)
	}
	return slices.Sorted(maps.Keys(object))
}

func TestPreviewJSON(t *testing.T) {
	db, s := openCatalog(t)
	body, err := json.Marshal(previewScore(t, s))
	if err != nil {
		t.Fatal(err)
	}
	var preview map[string]json.RawMessage
	if err := json.Unmarshal(body, &preview); err != nil {
		t.Fatal(err)
	}
	if got, want := jsonKeys(t, preview["evidence"]), []string{"favorites", "favorites_synced_at", "few_favorites", "new_favorites"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence keys = %v, want %v", got, want)
	}
	if got, want := jsonKeys(t, preview["recent"]), []string{"active", "always_hidden", "candidate", "never_hidden", "total"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("recent keys = %v, want %v", got, want)
	}
	if string(preview["no_model"]) != `"no_favorites"` || string(preview["candidate"]) != "null" || string(preview["overrides"]) != "[]" {
		t.Fatalf("preview JSON = %s", body)
	}

	retainRange(t, db, 1, 3, panda.Metadata{Tags: []string{"other:a"}})
	retain(t, db, panda.Metadata{ID: 4, Posted: 4}, true)
	favor(t, db, "42", 0, 1, 2, 3)
	body, err = json.Marshal(previewScore(t, s))
	if err != nil {
		t.Fatal(err)
	}
	var withCandidate struct {
		Candidate struct {
			Weights []json.RawMessage `json:"weights"`
			HeldOut json.RawMessage   `json:"held_out"`
		} `json:"candidate"`
	}
	if err := json.Unmarshal(body, &withCandidate); err != nil {
		t.Fatal(err)
	}
	if got, want := string(withCandidate.Candidate.HeldOut), `{"total":3,"always_hidden":0,"never_hidden":0,"scores":{"first_bucket":0,"counts":[3]}}`; got != want {
		t.Fatalf("held out = %s, want %s", got, want)
	}
	if len(withCandidate.Candidate.Weights) != 1 {
		t.Fatalf("preview JSON = %s", body)
	}
	if got, want := jsonKeys(t, withCandidate.Candidate.Weights[0]), []string{"active_weight", "catalog", "favorites", "kind", "label", "weight"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("weight keys = %v, want %v", got, want)
	}
}

func TestPreviewRejectsAnInvalidDefaultFilter(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1}, true)
	favor(t, db, "42", 0, 1)
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
	if got := scoreModelIDs(t, db); len(got) != 0 {
		t.Fatalf("models = %v", got)
	}
}
