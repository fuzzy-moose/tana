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
// weight, nil for a tag without one, its override rule and its weight's
// source.
type summaryTag struct {
	label    string
	weight   *float64
	override string
	source   string
}

func weighted(label string, weight float64) summaryTag {
	return summaryTag{label: label, weight: &weight}
}

func overridden(label, rule string) summaryTag { return summaryTag{label: label, override: rule} }

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
			tag.Override == want[i].override && tag.Source == want[i].source && (tag.Weight == nil) == (want[i].weight == nil) &&
			(tag.Weight == nil || math.Abs(*tag.Weight-*want[i].weight) <= 1e-9)
	}
	if !ok {
		got := make([]summaryTag, 0, len(item.SummaryTags))
		for _, tag := range item.SummaryTags {
			got = append(got, summaryTag{tag.Namespace + ":" + tag.Value, tag.Weight, tag.Override, tag.Source})
		}
		t.Fatalf("gallery %d summary tags = %v, want %v", item.GalleryID, describeTags(got), describeTags(want))
	}
}

func describeTags(tags []summaryTag) []string {
	described := make([]string, 0, len(tags))
	for _, tag := range tags {
		label := tag.label
		if tag.override != "" {
			label = tag.override + " " + label
		}
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

func TestSummaryTagsPutOverrideTagsFirstWithoutAModel(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 301, "artist:u1", "female:big hat", "male:hat")
	retainTagged(t, db, 302, "artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5", "artist:u6", "male:hat")

	setOverride(t, s, collectorapi.ScoreKindTag, "male:hat", collectorapi.ScoreRuleAlwaysHide)
	hatFirst := []summaryTag{overridden("male:hat", collectorapi.ScoreRuleAlwaysHide), {label: "artist:u1"}, {label: "female:big hat"}}
	for _, state := range []string{collectorapi.ScoreFilterAll, collectorapi.ScoreFilterHidden} {
		assertSummaryTags(t, itemByID(t, scorePage(t, s, state, collectorapi.CatalogOptions{}), 301), hatFirst)
	}
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 302),
		append([]summaryTag{overridden("male:hat", collectorapi.ScoreRuleAlwaysHide)},
			unweighted("artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5")...))

	setOverride(t, s, collectorapi.ScoreKindTag, "artist:u1", collectorapi.ScoreRuleAlwaysHide)
	setOverride(t, s, collectorapi.ScoreKindTag, "male:hat", collectorapi.ScoreRuleNeverHide)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), 301), []summaryTag{
		overridden("male:hat", collectorapi.ScoreRuleNeverHide), overridden("artist:u1", collectorapi.ScoreRuleAlwaysHide), {label: "female:big hat"}})

	setOverride(t, s, collectorapi.ScoreKindTag, "male:hat", collectorapi.ScoreRuleAlwaysHide)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 301), []summaryTag{
		overridden("artist:u1", collectorapi.ScoreRuleAlwaysHide), overridden("male:hat", collectorapi.ScoreRuleAlwaysHide), {label: "female:big hat"}})
}

func TestSummaryTagsIgnoreUploaderAndCategoryOverrides(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 303, Posted: 303, Uploader: "Bad One", Category: "Manga",
		Tags: []string{"male:hat", "artist:u6", "artist:u5", "artist:u4", "artist:u3", "artist:u2", "artist:u1"}}, true)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Bad One", collectorapi.ScoreRuleAlwaysHide)
	setOverride(t, s, collectorapi.ScoreKindCategory, "Manga", collectorapi.ScoreRuleNeverHide)
	item := itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 303)
	if item.Score == nil || item.Score.Override == "" {
		t.Fatalf("score = %+v", item.Score)
	}
	assertSummaryTags(t, item, unweighted("artist:u1", "artist:u2", "artist:u3", "artist:u4", "artist:u5", "artist:u6"))
}

func TestSummaryTagsPutOverrideTagsBeforeWeightedTags(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreGalleries(t, db)
	applyScoreModel(t, db, s, 0, storedScoreWeights()...)
	glasses, artist := glassesWeight, artistWeight
	retainTagged(t, db, 301, "artist:a", "female:glasses", "other:plain")
	retainTagged(t, db, 302, "artist:a", "female:glasses")

	setOverride(t, s, collectorapi.ScoreKindTag, "artist:a", collectorapi.ScoreRuleAlwaysHide)
	all := scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
	assertSummaryTags(t, itemByID(t, all, 302), []summaryTag{
		{label: "artist:a", weight: &artist, override: collectorapi.ScoreRuleAlwaysHide}, weighted("female:glasses", glasses)})

	setOverride(t, s, collectorapi.ScoreKindTag, "artist:a", collectorapi.ScoreRuleNeverHide)
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 302), []summaryTag{
		{label: "artist:a", weight: &artist, override: collectorapi.ScoreRuleNeverHide}, weighted("female:glasses", glasses)})

	var id int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM panda_score_overrides").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScoreOverride(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 302),
		[]summaryTag{weighted("female:glasses", glasses), weighted("artist:a", artist)})

	setOverride(t, s, collectorapi.ScoreKindTag, "other:plain", collectorapi.ScoreRuleNeverHide)
	plainFirst := []summaryTag{overridden("other:plain", collectorapi.ScoreRuleNeverHide), weighted("female:glasses", glasses), weighted("artist:a", artist)}
	assertSummaryTags(t, itemByID(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), 301), plainFirst)
	assertSummaryTags(t, itemByID(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), 301), plainFirst)
}
