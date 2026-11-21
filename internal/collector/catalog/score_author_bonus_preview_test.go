package catalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// setFavoriteAuthor puts the catalog tag namespace:value into
// favorite_authors with the number of favorites.
func setFavoriteAuthor(t *testing.T, db *pgxpool.Pool, label string, favorites int) {
	t.Helper()
	namespace, value, _ := strings.Cut(label, ":")
	tag, err := db.Exec(t.Context(), `INSERT INTO favorite_authors (tag_id, favorites)
		SELECT id, $3 FROM catalog_tags WHERE namespace = $1 AND value = $2`, namespace, value, favorites)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatalf("favorite author %s: %v, %d rows", label, err, tag.RowsAffected())
	}
}

func float(value float64) *float64 { return &value }

func TestPreviewFavoriteAuthorsWithoutACandidate(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 1, "artist:a", "artist:b")
	setFavoriteAuthor(t, db, "artist:a", 2)
	setFavoriteAuthor(t, db, "artist:b", 2)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:b", -1)

	want := []collectorapi.ScoreFavoriteAuthor{{Label: "artist:a", Favorites: 2}, {Label: "artist:b", Favorites: 2, Weight: float(-1)}}
	preview := previewScore(t, s)
	if preview.Candidate != nil || !reflect.DeepEqual(preview.FavoriteAuthors, want) {
		t.Fatalf("candidate %v, authors %s", preview.Candidate, authorsString(preview.FavoriteAuthors))
	}
	setBonus(t, s, float(2))
	want[0].Weight = float(2)
	if got := previewScore(t, s).FavoriteAuthors; !reflect.DeepEqual(got, want) {
		t.Fatalf("authors %s, want %s", authorsString(got), authorsString(want))
	}
}

func TestPreviewListsThe500AuthorsWithTheMostFavorites(t *testing.T) {
	db, s := openCatalog(t)
	if _, err := db.Exec(t.Context(), `WITH tags AS (
			INSERT INTO catalog_tags (namespace, value, value_lower)
			SELECT 'artist', 'a' || lpad(i::text, 3, '0'), 'a' || lpad(i::text, 3, '0') FROM generate_series(1, 501) i
			RETURNING id, value)
		INSERT INTO favorite_authors (tag_id, favorites)
		SELECT id, CASE WHEN value = 'a001' THEN 1 ELSE 2 END FROM tags`); err != nil {
		t.Fatal(err)
	}
	preview := previewScore(t, s)
	if preview.FavoriteAuthorCount != 501 || len(preview.FavoriteAuthors) != scorePreviewAuthors {
		t.Fatalf("count %d, listed %d", preview.FavoriteAuthorCount, len(preview.FavoriteAuthors))
	}
	if first, last := preview.FavoriteAuthors[0], preview.FavoriteAuthors[scorePreviewAuthors-1]; first.Label != "artist:a002" || last.Label != "artist:a501" {
		t.Fatalf("first %+v, last %+v", first, last)
	}
}

func authorsString(authors []collectorapi.ScoreFavoriteAuthor) string {
	var parts []string
	for _, author := range authors {
		weight := "nil"
		if author.Weight != nil {
			body, _ := json.Marshal(*author.Weight)
			weight = string(body)
		}
		body, _ := json.Marshal(author.Favorites)
		parts = append(parts, author.Label+"/"+string(body)+"/"+weight)
	}
	return strings.Join(parts, " ")
}
