package catalog

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// summaryTag is an expected summary tag: its namespace:value label, its
// weight, nil for a tag without one, and its weight's source.
type summaryTag struct {
	label  string
	weight *float64
	source string
}

func weighted(label string, weight float64) summaryTag {
	return summaryTag{label: label, weight: &weight}
}

func unweighted(labels ...string) []summaryTag {
	tags := make([]summaryTag, 0, len(labels))
	for _, label := range labels {
		tags = append(tags, summaryTag{label: label})
	}
	return tags
}

func assertSummaryTags(t *testing.T, item collectorapi.CatalogItem, want []summaryTag) {
	t.Helper()
	ok := item.SummaryTags != nil && len(item.SummaryTags) == len(want)
	for i := 0; ok && i < len(want); i++ {
		tag := item.SummaryTags[i]
		ok = tag.Namespace+":"+tag.Value == want[i].label && tag.Term == gallerysearch.SuggestionTerm("", tag.Namespace, tag.Value) &&
			tag.Source == want[i].source && (tag.Weight == nil) == (want[i].weight == nil) &&
			(tag.Weight == nil || math.Abs(*tag.Weight-*want[i].weight) <= 1e-9)
	}
	if !ok {
		got := make([]summaryTag, 0, len(item.SummaryTags))
		for _, tag := range item.SummaryTags {
			got = append(got, summaryTag{tag.Namespace + ":" + tag.Value, tag.Weight, tag.Source})
		}
		t.Fatalf("gallery %d summary tags = %v, want %v", item.GalleryID, describeTags(got), describeTags(want))
	}
}

func describeTags(tags []summaryTag) []string {
	described := make([]string, 0, len(tags))
	for _, tag := range tags {
		label := tag.label
		if tag.weight != nil {
			label += "=" + strconv.FormatFloat(*tag.weight, 'g', 4, 64)
		}
		if tag.source != "" {
			label += " (" + tag.source + ")"
		}
		described = append(described, label)
	}
	return described
}

func retainTagged(t *testing.T, db *pgxpool.Pool, id int64, tags ...string) {
	t.Helper()
	retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: tags}, true)
}

func TestSummaryTagsRankWeightedTagsFirst(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	seedScoredGalleries(t, db)
	artistsAF := unweighted(scoreArtists...)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 104), artistsAF)

	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	glasses, artist, hat := glassesWeight, artistWeight, hatWeight
	ranked104 := []summaryTag{weighted("female:glasses", glasses)}
	for _, label := range scoreArtists[:5] {
		ranked104 = append(ranked104, weighted(label, artist))
	}

	retainTagged(t, db, 301, "artist:a", "artist:u1", "male:hat")
	retainTagged(t, db, 302, "artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5", "artist:u6", "male:hat")
	retainTagged(t, db, 303, "artist:u7", "artist:u6", "artist:u5", "artist:u4", "artist:u3", "artist:u2", "artist:u1")
	retain(t, db, panda.Metadata{ID: 304, Posted: 304, Uploader: "Bad One", Category: "Manga", Tags: []string{"artist:u1", "male:hat"}}, true)
	retainTagged(t, db, 306)

	all := scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	ranked301 := []summaryTag{weighted("male:hat", hat), weighted("artist:a", artist), {label: "artist:u1"}}
	assertSummaryTags(t, itemByID(t, all, 104), ranked104)
	assertSummaryTags(t, itemByID(t, all, 301), ranked301)
	assertSummaryTags(t, itemByID(t, all, 302), append([]summaryTag{weighted("male:hat", hat)},
		unweighted("artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5")...))
	assertSummaryTags(t, itemByID(t, all, 303), unweighted("artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5", "artist:u6"))
	assertSummaryTags(t, itemByID(t, all, 304), []summaryTag{weighted("male:hat", hat), {label: "artist:u1"}})
	assertSummaryTags(t, itemByID(t, all, 306), []summaryTag{})

	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), 104), ranked104)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 301), ranked301)
	assertSummaryTags(t, itemByID(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), 301), ranked301)

	if err := s.SetScoreThreshold(t.Context(), -100); err != nil {
		t.Fatal(err)
	}
	assertSummaryTags(t, itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 104), ranked104)
	if err := s.RemoveScoreModel(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertSummaryTags(t, itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 104), artistsAF)
}

func TestSummaryTagsRankProvisionalThenCollectedTags(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	glasses, hat := glassesWeight, hatWeight
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 201, Token: "token201"}, Title: "Feed",
		Tags: []string{"artist:u1", "male:hat"}}, time.Now().UTC())
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 201),
		[]summaryTag{weighted("male:hat", hat), {label: "artist:u1"}})
	retain(t, db, panda.Metadata{ID: 201, Posted: 201, Title: "Feed", Tags: []string{"artist:u2", "female:glasses"}}, true)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 201),
		[]summaryTag{weighted("female:glasses", glasses), {label: "artist:u2"}})
}

func TestSummaryTagsKeepNamespaceOrderWithoutAModel(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 302, "male:hat", "artist:u6", "artist:u5", "artist:u4", "artist:u3", "artist:u2", "artist:u1")
	setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 5)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 302),
		unweighted("artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5", "artist:u6"))
}
