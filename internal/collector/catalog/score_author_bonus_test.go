package catalog

import (
	"errors"
	"math"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// makeFavoriteAuthor puts the catalog tag namespace:value into
// favorite_authors with two favorites.
func makeFavoriteAuthor(t *testing.T, db *pgxpool.Pool, label string) {
	t.Helper()
	setFavoriteAuthor(t, db, label, 2)
}

func setBonus(t *testing.T, s *Service, bonus *float64) {
	t.Helper()
	if err := s.SetAuthorBonus(t.Context(), bonus); err != nil {
		t.Fatal(err)
	}
}

func storedBonus(t *testing.T, db *pgxpool.Pool) *float64 {
	t.Helper()
	var bonus *float64
	if err := db.QueryRow(t.Context(), "SELECT author_bonus FROM panda_score_settings WHERE id = 1").Scan(&bonus); err != nil {
		t.Fatal(err)
	}
	return bonus
}

// favoriteAuthorLabels returns the labels of the item's summary tags that are
// marked as favorite authors, sorted.
func favoriteAuthorLabels(item collectorapi.CatalogItem) []string {
	labels := []string{}
	for _, tag := range item.SummaryTags {
		if tag.FavoriteAuthor {
			labels = append(labels, tag.Namespace+":"+tag.Value)
		}
	}
	slices.Sort(labels)
	return labels
}

func bonusTag(label string, weight float64) summaryTag {
	return summaryTag{label: label, weight: &weight, source: collectorapi.ScoreSourceAuthorBonus}
}

func TestSetAuthorBonusStoresANumberAboveZero(t *testing.T) {
	db, s := openCatalog(t)
	if bonus := storedBonus(t, db); bonus != nil {
		t.Fatalf("initial bonus = %v", *bonus)
	}
	bonus := 2.5
	setBonus(t, s, &bonus)
	for _, invalid := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1.1e300} {
		if err := s.SetAuthorBonus(t.Context(), &invalid); !errors.Is(err, ErrInvalidAuthorBonus) {
			t.Fatalf("bonus %g: %v", invalid, err)
		}
	}
	if got := storedBonus(t, db); got == nil || *got != 2.5 {
		t.Fatalf("bonus after invalid = %v", got)
	}
	largest := maxScoreWeight
	setBonus(t, s, &largest)
	if got := storedBonus(t, db); got == nil || *got != maxScoreWeight {
		t.Fatalf("bonus = %v", got)
	}
	setBonus(t, s, nil)
	if got := storedBonus(t, db); got != nil {
		t.Fatalf("bonus after off = %v", *got)
	}
}

// goodWeight is the weight of artist:good in the model of
// seedBonusGalleries.
var goodWeight = math.Log(2.5)

// seedBonusGalleries applies a model with storedScoreWeights and artist:good,
// and retains galleries 105 with artist:a, 106 with artist:rare and
// female:glasses, 107 with artist:b and 108 with artist:good. artist:a,
// artist:rare and artist:good are favorite authors; artist:rare has no
// calculated weight.
func seedBonusGalleries(t *testing.T, db *pgxpool.Pool, s *Service) {
	t.Helper()
	seedScoreGalleries(t, db)
	retainTagged(t, db, 105, "artist:a")
	retainTagged(t, db, 106, "artist:rare", "female:glasses")
	retainTagged(t, db, 107, "artist:b")
	retainTagged(t, db, 108, "artist:good")
	applyScoreModel(t, db, s, -3, append(storedScoreWeights(), storedWeight{collectorapi.ScoreKindTag, "artist:good", goodWeight})...)
	for _, label := range []string{"artist:a", "artist:rare", "artist:good"} {
		makeFavoriteAuthor(t, db, label)
	}
}

func TestAuthorBonusLiftsFavoriteAuthorsInTheNextPage(t *testing.T) {
	db, s := openCatalog(t)
	seedBonusGalleries(t, db, s)
	artist, glasses, good := artistWeight, glassesWeight, goodWeight
	assertCalculated := func() {
		t.Helper()
		assertListed(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 108)
		page := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
		assertListed(t, page, 105, 106, 107)
		assertWeights(t, itemByID(t, page, 105), []collectorapi.CatalogScoreWeight{
			{Kind: collectorapi.ScoreKindTag, Label: "artist:a", Weight: artist}})
		assertSummaryTags(t, itemByID(t, page, 106), []summaryTag{weighted("female:glasses", glasses), {label: "artist:rare"}})
	}
	assertCalculated()

	bonus := 2.0
	setBonus(t, s, &bonus)
	page := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertListed(t, page, 105, 106, 108)
	item := itemByID(t, page, 105)
	assertScoreValue(t, item, 2)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "artist:a", Weight: 2, Source: collectorapi.ScoreSourceAuthorBonus}})
	assertSummaryTags(t, item, []summaryTag{bonusTag("artist:a", 2)})
	item = itemByID(t, page, 106)
	assertScoreValue(t, item, 2+glasses)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "artist:rare", Weight: 2, Source: collectorapi.ScoreSourceAuthorBonus},
		{Kind: collectorapi.ScoreKindTag, Label: "female:glasses", Weight: glasses}})
	assertSummaryTags(t, item, []summaryTag{bonusTag("artist:rare", 2), weighted("female:glasses", glasses)})
	assertScoreValue(t, itemByID(t, page, 108), 2)
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertListed(t, hidden, 107)
	assertWeights(t, itemByID(t, hidden, 107), []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "artist:b", Weight: artist}})

	below := good / 2
	setBonus(t, s, &below)
	page = scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	item = itemByID(t, page, 108)
	assertScoreValue(t, item, good)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "artist:good", Weight: good}})
	assertSummaryTags(t, item, []summaryTag{weighted("artist:good", good)})
	assertScoreValue(t, itemByID(t, page, 105), below)

	setBonus(t, s, nil)
	assertCalculated()
}

func TestPinWinsOverTheAuthorBonus(t *testing.T) {
	db, s := openCatalog(t)
	seedBonusGalleries(t, db, s)
	bonus := 2.0
	setBonus(t, s, &bonus)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:a", -1)
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertListed(t, hidden, 105, 107)
	item := itemByID(t, hidden, 105)
	assertScoreValue(t, item, -1)
	assertWeights(t, item, []collectorapi.CatalogScoreWeight{
		{Kind: collectorapi.ScoreKindTag, Label: "artist:a", Weight: -1, Source: collectorapi.ScoreSourcePin}})
	assertSummaryTags(t, item, []summaryTag{pinnedTag("artist:a", -1)})
	if got := favoriteAuthorLabels(item); !slices.Equal(got, []string{"artist:a"}) {
		t.Fatalf("favorite authors = %v", got)
	}
}

func TestAuthorBonusHasNoEffectWithoutAnActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 105, "artist:a")
	retainTagged(t, db, 106, "artist:rare", "female:glasses")
	makeFavoriteAuthor(t, db, "artist:a")
	makeFavoriteAuthor(t, db, "artist:rare")
	bonus := 2.0
	setBonus(t, s, &bonus)
	assertNoBonus := func(page collectorapi.CatalogResult) {
		t.Helper()
		for _, item := range page.Items {
			for _, tag := range item.SummaryTags {
				if tag.Weight != nil || tag.Source != "" || tag.FavoriteAuthor {
					t.Fatalf("gallery %d summary tag = %+v", item.GalleryID, tag)
				}
			}
		}
	}
	page := scorePage(t, s, "", collectorapi.CatalogOptions{})
	if page.ScoreFilter != nil || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	for _, item := range page.Items {
		if item.Score != nil {
			t.Fatalf("gallery %d score = %+v", item.GalleryID, item.Score)
		}
	}
	assertNoBonus(page)
}

func TestSummaryTagsMarkFavoriteAuthors(t *testing.T) {
	db, s := openCatalog(t)
	seedBonusGalleries(t, db, s)
	retainTagged(t, db, 109, "artist:a", "artist:b", "artist:good", "artist:rare", "female:glasses")
	setPin(t, s, collectorapi.ScoreKindTag, "artist:rare", 0.1)
	want := []string{"artist:a", "artist:good", "artist:rare"}
	assertMarks := func() {
		t.Helper()
		item := itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 109)
		if got := favoriteAuthorLabels(item); !slices.Equal(got, want) {
			t.Fatalf("favorite authors = %v, want %v", got, want)
		}
	}
	assertMarks()
	bonus := 2.0
	setBonus(t, s, &bonus)
	assertMarks()
}
