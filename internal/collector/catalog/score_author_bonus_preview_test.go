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

func TestPreviewCarriesTheAuthorBonusAndTheFavoriteAuthors(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	retainTagged(t, db, 201, "artist:Zed")
	preview := previewScore(t, s, false)
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"author_bonus":null`, `"favorite_author_count":0`, `"favorite_authors":[]`} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("preview JSON lacks %s: %s", want, body)
		}
	}

	hat := weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat")
	artist := weightOf(t, preview, collectorapi.ScoreKindTag, "artist:a")
	if hat <= 0.1 || artist >= 0.1 {
		t.Fatalf("male:hat %g, artist:a %g", hat, artist)
	}
	setFavoriteAuthor(t, db, "female:glasses", 1)
	setFavoriteAuthor(t, db, "male:hat", 3)
	setFavoriteAuthor(t, db, "artist:Zed", 3)
	setFavoriteAuthor(t, db, "artist:a", 5)
	setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", -5)

	preview = previewScore(t, s, false)
	want := []collectorapi.ScoreFavoriteAuthor{
		{Label: "artist:a", Favorites: 5, Weight: float(weightOf(t, preview, collectorapi.ScoreKindTag, "artist:a"))},
		{Label: "artist:Zed", Favorites: 3},
		{Label: "male:hat", Favorites: 3, Weight: float(weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat"))},
		{Label: "female:glasses", Favorites: 1, Weight: float(-5)},
	}
	if preview.AuthorBonus != nil || preview.FavoriteAuthorCount != 4 || !reflect.DeepEqual(preview.FavoriteAuthors, want) {
		t.Fatalf("bonus %v, count %d, authors %s, want %s", preview.AuthorBonus, preview.FavoriteAuthorCount, authorsString(preview.FavoriteAuthors), authorsString(want))
	}

	setBonus(t, s, float(0.1))
	preview = previewScore(t, s, true)
	want[0].Weight = float(0.1)
	want[1].Weight = float(0.1)
	want[2].Weight = float(weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat"))
	if preview.AuthorBonus == nil || *preview.AuthorBonus != 0.1 || preview.FavoriteAuthorCount != 4 || !reflect.DeepEqual(preview.FavoriteAuthors, want) {
		t.Fatalf("bonus %v, count %d, authors %s, want %s", preview.AuthorBonus, preview.FavoriteAuthorCount, authorsString(preview.FavoriteAuthors), authorsString(want))
	}
}

func TestPreviewFavoriteAuthorsWithoutACandidate(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 1, "artist:a", "artist:b")
	setFavoriteAuthor(t, db, "artist:a", 2)
	setFavoriteAuthor(t, db, "artist:b", 2)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:b", -1)

	want := []collectorapi.ScoreFavoriteAuthor{{Label: "artist:a", Favorites: 2}, {Label: "artist:b", Favorites: 2, Weight: float(-1)}}
	preview := previewScore(t, s, false)
	if preview.Candidate != nil || !reflect.DeepEqual(preview.FavoriteAuthors, want) {
		t.Fatalf("candidate %v, authors %s", preview.Candidate, authorsString(preview.FavoriteAuthors))
	}
	setBonus(t, s, float(2))
	want[0].Weight = float(2)
	if got := previewScore(t, s, false).FavoriteAuthors; !reflect.DeepEqual(got, want) {
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
	preview := previewScore(t, s, false)
	if preview.FavoriteAuthorCount != 501 || len(preview.FavoriteAuthors) != scorePreviewAuthors {
		t.Fatalf("count %d, listed %d", preview.FavoriteAuthorCount, len(preview.FavoriteAuthors))
	}
	if first, last := preview.FavoriteAuthors[0], preview.FavoriteAuthors[scorePreviewAuthors-1]; first.Label != "artist:a002" || last.Label != "artist:a501" {
		t.Fatalf("first %+v, last %+v", first, last)
	}
}

func TestHeldOutScoresCountTheAuthorBonus(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	makeFavoriteAuthor(t, db, "artist:a")
	before := histogramCounts(t, &previewScore(t, s, false).Candidate.HeldOut.NotForMe.Scores)

	// No fold weighs artist:a, which galleries 1 to 3 carry, so the bonus
	// gives its weight there.
	setBonus(t, s, float(100))
	for _, favorites := range []bool{false, true} {
		after := histogramCounts(t, &previewScore(t, s, favorites).Candidate.HeldOut.NotForMe.Scores)
		var shifted, sumBefore, sumAfter int
		for bucket, count := range before {
			sumBefore += bucket * count
		}
		for bucket, count := range after {
			sumAfter += bucket * count
			if bucket >= 900 {
				shifted += count
			}
		}
		if shifted != 3 || sumAfter-sumBefore < 2997 || sumAfter-sumBefore > 3003 {
			t.Fatalf("favorites %v: counts %v, before %v", favorites, after, before)
		}
	}

	setBonus(t, s, nil)
	if got := histogramCounts(t, &previewScore(t, s, false).Candidate.HeldOut.NotForMe.Scores); !reflect.DeepEqual(got, before) {
		t.Fatalf("counts without bonus %v, want %v", got, before)
	}
}

func TestPreviewRecentCountsTheAuthorBonus(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	makeFavoriteAuthor(t, db, "artist:a")
	setBonus(t, s, float(100))
	preview := previewScore(t, s, false)
	// Gallery 104 carries female:glasses and the artist tags.
	score := 100 + weightOf(t, preview, collectorapi.ScoreKindTag, "female:glasses")
	for _, label := range scoreArtists[1:] {
		score += weightOf(t, preview, collectorapi.ScoreKindTag, label)
	}
	if got, want := histogramCounts(t, preview.Recent.Candidate), map[int]int{-7: 1, 6: 1, 0: 1, tenths(score): 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate counts = %v, want %v", got, want)
	}

	setBonus(t, s, nil)
	if got, want := histogramCounts(t, previewScore(t, s, false).Recent.Candidate), map[int]int{-7: 1, 6: 1, 0: 1, -36: 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidate counts = %v, want %v", got, want)
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
