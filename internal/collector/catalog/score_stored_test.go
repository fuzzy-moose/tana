package catalog

import (
	"math"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// storedWeight is a weight that a test stores with a model: a tag by its
// namespace:value label, an uploader or a category by its stored value.
type storedWeight struct {
	kind   string
	label  string
	weight float64
}

// The weights of storedScoreWeights.
var (
	glassesWeight  = math.Log(0.5)
	artistWeight   = math.Log(0.625)
	uploaderWeight = math.Log(0.625)
	hatWeight      = math.Log(2)
	mangaWeight    = math.Log(1.6)
)

// storedScoreWeights returns the weights of a model over the galleries of
// seedScoreGalleries: female:glasses, each artist tag, the uploader "Bad One",
// male:hat and the category manga.
func storedScoreWeights() []storedWeight {
	weights := []storedWeight{
		{collectorapi.ScoreKindTag, "female:glasses", glassesWeight},
		{collectorapi.ScoreKindTag, "male:hat", hatWeight},
		{collectorapi.ScoreKindUploader, "Bad One", uploaderWeight},
		{collectorapi.ScoreKindCategory, "manga", mangaWeight},
	}
	for _, label := range scoreArtists {
		weights = append(weights, storedWeight{collectorapi.ScoreKindTag, label, artistWeight})
	}
	return weights
}

var scoreArtists = []string{"artist:a", "artist:b", "artist:c", "artist:d", "artist:e", "artist:f"}

// seedScoreGalleries retains galleries 1 to 5 with female:glasses, 1 to 3
// also with six artist tags and the uploader "Bad One", and galleries 6 to 10
// with male:hat, 6 to 8 also in the category Manga. Under storedScoreWeights
// galleries 1 to 3 score tenths -40, 4 and 5 score -7, 6 to 8 score 11 and 9
// and 10 score 6.
func seedScoreGalleries(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	retainRange := func(first int64, count int, uploader, category string, tags ...string) {
		for id := first; id < first+int64(count); id++ {
			retain(t, db, panda.Metadata{ID: id, Posted: id, Uploader: uploader, Category: category, Tags: tags}, true)
		}
	}
	retainRange(1, 3, "Bad One", "", append([]string{"female:glasses"}, scoreArtists...)...)
	retainRange(4, 2, "", "", "female:glasses")
	retainRange(6, 3, "", "Manga", "male:hat")
	retainRange(9, 2, "", "", "male:hat")
}

// storeScoreCandidate stores a candidate with the weights, as a preview
// would, and returns its ID. Each tag must be in the catalog.
func storeScoreCandidate(t *testing.T, db *pgxpool.Pool, weights ...storedWeight) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(t.Context(), `INSERT INTO panda_score_models (calculated_at, favorites_in_scope, catalog_in_scope)
		VALUES (now(), 0, 0) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, weight := range weights {
		var tagID *int64
		value := weight.label
		if weight.kind == collectorapi.ScoreKindTag {
			namespace, tag, _ := strings.Cut(weight.label, ":")
			if err := db.QueryRow(t.Context(), "SELECT id FROM catalog_tags WHERE namespace = $1 AND value_lower = $2",
				namespace, strings.ToLower(tag)).Scan(&tagID); err != nil {
				t.Fatalf("tag %s: %v", weight.label, err)
			}
			value = ""
		}
		if _, err := db.Exec(t.Context(), `INSERT INTO panda_score_weights (model_id, kind, tag_id, value, weight, favorites, catalog)
			VALUES ($1, $2, $3, $4, $5, 0, 0)`, id, weight.kind, tagID, value, weight.weight); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

// applyScoreModel stores a candidate with the weights and applies it with the
// threshold, and returns its ID.
func applyScoreModel(t *testing.T, db *pgxpool.Pool, s *Service, threshold int, weights ...storedWeight) int64 {
	t.Helper()
	id := storeScoreCandidate(t, db, weights...)
	if err := s.ApplyScoreModel(t.Context(), id, threshold); err != nil {
		t.Fatal(err)
	}
	return id
}
