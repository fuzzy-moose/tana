package catalog

import (
	"errors"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func setOverride(t *testing.T, s *Service, kind, value, rule string) {
	t.Helper()
	if err := s.SetScoreOverride(t.Context(), kind, value, rule); err != nil {
		t.Fatal(err)
	}
}

func scorePage(t *testing.T, s *Service, filter string, options collectorapi.CatalogOptions) collectorapi.CatalogResult {
	t.Helper()
	options.ScoreFilter = filter
	if options.PageSize == 0 {
		options.PageSize = 100
	}
	result, err := s.List(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func itemByID(t *testing.T, result collectorapi.CatalogResult, id int64) collectorapi.CatalogItem {
	t.Helper()
	for _, item := range result.Items {
		if item.GalleryID == id {
			return item
		}
	}
	t.Fatalf("gallery %d not listed: %+v", id, result.Items)
	return collectorapi.CatalogItem{}
}

func assertScore(t *testing.T, item collectorapi.CatalogItem, rule, override string) {
	t.Helper()
	if rule == "" {
		if item.Score != nil {
			t.Fatalf("gallery %d score = %+v, want none", item.GalleryID, item.Score)
		}
		return
	}
	if item.Score == nil || item.Score.Rule != rule || item.Score.Override != override {
		t.Fatalf("gallery %d score = %+v, want %s %q", item.GalleryID, item.Score, rule, override)
	}
}

func TestScoreFilterIsNotInForceWithoutOverrides(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 3; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: []string{"female:big hat"}}, true)
	}
	for _, filter := range []string{"", collectorapi.ScoreFilterAll} {
		for _, unseen := range []bool{false, true} {
			result := scorePage(t, s, filter, collectorapi.CatalogOptions{Unseen: unseen})
			assertCatalogIDs(t, result, []int64{3, 2, 1})
			if result.ScoreFilter != nil {
				t.Fatalf("score filter = %+v", result.ScoreFilter)
			}
			for _, item := range result.Items {
				assertScore(t, item, "", "")
			}
		}
	}
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{})
}

func TestScoreFilterHidesOverrideMatches(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Posted: 1, Tags: []string{"female:big hat"}, Uploader: "Other", Category: "Doujinshi"},
		{ID: 2, Posted: 2, Uploader: "Some One", Category: "Doujinshi"},
		{ID: 3, Posted: 3, Uploader: "Other", Category: "Manga"},
		{ID: 4, Posted: 4},
		{ID: 5, Posted: 5, Tags: []string{"female:big hat"}, Uploader: "Some One", Category: "Manga"},
		{ID: 6, Posted: 6, Uploader: "Other", Category: "Doujinshi"},
	} {
		retain(t, db, entry, true)
	}
	for _, tc := range []struct {
		kind, value, label string
		hidden             []int64
	}{
		{collectorapi.ScoreKindTag, "female:big_hat", "female:big hat", []int64{5, 1}},
		{collectorapi.ScoreKindUploader, "Some One", "Some One", []int64{5, 2}},
		{collectorapi.ScoreKindCategory, " Manga ", "manga", []int64{5, 3}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			setOverride(t, s, tc.kind, tc.value, collectorapi.ScoreRuleAlwaysHide)
			defer func() {
				if _, err := db.Exec(t.Context(), "DELETE FROM panda_score_overrides"); err != nil {
					t.Fatal(err)
				}
			}()
			isHidden := map[int64]bool{}
			for _, id := range tc.hidden {
				isHidden[id] = true
			}
			shown := []int64{}
			for id := int64(6); id >= 1; id-- {
				if !isHidden[id] {
					shown = append(shown, id)
				}
			}
			on := scorePage(t, s, "", collectorapi.CatalogOptions{})
			assertCatalogIDs(t, on, shown)
			hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
			assertCatalogIDs(t, hidden, tc.hidden)
			all := scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{})
			assertCatalogIDs(t, all, []int64{6, 5, 4, 3, 2, 1})
			for _, result := range []collectorapi.CatalogResult{on, hidden, all} {
				if result.ScoreFilter == nil || result.ScoreFilter.HiddenUnseen {
					t.Fatalf("score filter = %+v", result.ScoreFilter)
				}
				for _, item := range result.Items {
					if isHidden[item.GalleryID] {
						assertScore(t, item, collectorapi.ScoreRuleAlwaysHide, tc.label)
					} else {
						assertScore(t, item, "", "")
					}
				}
			}
		})
	}
}

func TestScoreFilterRuleOrder(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Posted: 1, Tags: []string{"female:glasses"}},
		{ID: 2, Posted: 2, Tags: []string{"female:big hat", "male:hat"}},
		{ID: 3, Posted: 3, Tags: []string{"female:big hat", "female:glasses"}, Uploader: "Some One"},
		{ID: 4, Posted: 4, Tags: []string{"male:hat"}},
	} {
		retain(t, db, entry, true)
	}
	applyScoreModel(t, db, s, 0,
		storedWeight{collectorapi.ScoreKindTag, "female:glasses", -2}, storedWeight{collectorapi.ScoreKindTag, "male:hat", 2})
	setOverride(t, s, collectorapi.ScoreKindTag, "female:big hat", collectorapi.ScoreRuleAlwaysHide)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleNeverHide)

	on := scorePage(t, s, "", collectorapi.CatalogOptions{})
	assertCatalogIDs(t, on, []int64{4, 3})
	assertScore(t, itemByID(t, on, 3), collectorapi.ScoreRuleNeverHide, "Some One")
	if score := itemByID(t, on, 4).Score; score == nil || score.Rule != "" {
		t.Fatalf("gallery 4 score = %+v", score)
	}
	hidden := scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{})
	assertCatalogIDs(t, hidden, []int64{2, 1})
	assertScore(t, itemByID(t, hidden, 2), collectorapi.ScoreRuleAlwaysHide, "female:big hat")
	assertScore(t, itemByID(t, hidden, 1), collectorapi.ScoreRuleBelowThreshold, "")
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterAll, collectorapi.CatalogOptions{}), []int64{4, 3, 2, 1})
}

func TestScoreFilterPagesThroughCategoryBranches(t *testing.T) {
	db, s := openCatalog(t)
	want := []int64{}
	for id := int64(1); id <= 14; id++ {
		entry := panda.Metadata{ID: id, Posted: id, Category: "Manga"}
		if id%2 == 0 {
			entry.Category = "Doujinshi"
		}
		if id > 7 || id == 3 {
			entry.Tags = []string{"female:big hat"}
		} else {
			want = append([]int64{id}, want...)
		}
		retain(t, db, entry, true)
	}
	retain(t, db, panda.Metadata{ID: 15, Posted: 15, Category: "Cosplay"}, true)
	setOverride(t, s, collectorapi.ScoreKindTag, "female:big hat", collectorapi.ScoreRuleAlwaysHide)
	options := collectorapi.CatalogOptions{Categories: []string{"manga", "doujinshi"}, PageSize: 4}
	first := scorePage(t, s, "", options)
	assertCatalogIDs(t, first, want[:4])
	got := []int64{}
	for result := first; ; {
		for _, item := range result.Items {
			got = append(got, item.GalleryID)
		}
		if result.NextCursor == "" {
			break
		}
		options.Cursor = result.NextCursor
		result = scorePage(t, s, "", options)
	}
	assertCatalogIDs(t, collectorapi.CatalogResult{Items: idItems(got)}, want)
}

func idItems(ids []int64) []collectorapi.CatalogItem {
	items := make([]collectorapi.CatalogItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, collectorapi.CatalogItem{GalleryID: id})
	}
	return items
}

func TestScoreFilterAppliesAfterTheDefaultFilter(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Posted: 1, Category: "Manga"},
		{ID: 2, Posted: 2, Category: "Manga", Uploader: "Some One"},
		{ID: 3, Posted: 3, Category: "Doujinshi"},
		{ID: 4, Posted: 4, Category: "Doujinshi", Uploader: "Some One"},
	} {
		retain(t, db, entry, true)
	}
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleAlwaysHide)
	options := collectorapi.CatalogOptions{DefaultCategories: []string{"manga"}}
	assertCatalogIDs(t, scorePage(t, s, "", options), []int64{1})
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, options), []int64{2})
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterAll, options), []int64{2, 1})
}

func TestScoreFilterInTheUnseenView(t *testing.T) {
	db, s := openCatalog(t)
	for _, entry := range []panda.Metadata{
		{ID: 1, Posted: 1, Title: "red", Uploader: "Some One"},
		{ID: 2, Posted: 2, Title: "blue"},
		{ID: 3, Posted: 3, Title: "red", Uploader: "Some One"},
		{ID: 4, Posted: 4, Title: "green"},
	} {
		retain(t, db, entry, true)
	}
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleAlwaysHide)
	page := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	assertCatalogIDs(t, page, []int64{4, 2})
	if page.ScoreFilter == nil || page.ScoreFilter.HiddenUnseen {
		t.Fatalf("non-empty page score filter = %+v", page.ScoreFilter)
	}
	walked := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100, Cursor: page.NextCursor})
	assertCatalogIDs(t, walked, []int64{})
	if walked.ScoreFilter == nil || walked.ScoreFilter.HiddenUnseen {
		t.Fatalf("page with a cursor score filter = %+v", walked.ScoreFilter)
	}
	if err := s.RecordDisplays(t.Context(), []int64{4, 2}); err != nil {
		t.Fatal(err)
	}
	empty := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100})
	assertCatalogIDs(t, empty, []int64{})
	if empty.ScoreFilter == nil || !empty.ScoreFilter.HiddenUnseen {
		t.Fatalf("empty page score filter = %+v", empty.ScoreFilter)
	}
	searched := unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100, Query: "blue"})
	assertCatalogIDs(t, searched, []int64{})
	if searched.ScoreFilter == nil || searched.ScoreFilter.HiddenUnseen {
		t.Fatalf("searched page score filter = %+v", searched.ScoreFilter)
	}
	for _, filter := range []string{collectorapi.ScoreFilterAll, collectorapi.ScoreFilterHidden} {
		result := scorePage(t, s, filter, collectorapi.CatalogOptions{Unseen: true, Query: "blue"})
		if len(result.Items) != 0 || result.ScoreFilter == nil || result.ScoreFilter.HiddenUnseen {
			t.Fatalf("%s page = %+v", filter, result)
		}
	}
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{Unseen: true}), []int64{3, 1})
	if _, err := db.Exec(t.Context(), "DELETE FROM panda_score_overrides"); err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, unseenPage(t, s, collectorapi.CatalogOptions{PageSize: 100}), []int64{3, 1})
}

func TestCatchUpCoversHiddenGalleries(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Uploader: "Some One"}, true)
	retain(t, db, panda.Metadata{ID: 2, Posted: 2}, true)
	setOverride(t, s, collectorapi.ScoreKindUploader, "Some One", collectorapi.ScoreRuleAlwaysHide)
	catchUp(t, s, 2)
	for _, filter := range []string{"", collectorapi.ScoreFilterHidden} {
		assertCatalogIDs(t, scorePage(t, s, filter, collectorapi.CatalogOptions{Unseen: true}), []int64{})
	}
}

func TestScoreFilterRejectsInvalidStates(t *testing.T) {
	_, s := openCatalog(t)
	if _, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, ScoreFilter: "bogus"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("err = %v", err)
	}
}

func TestScoreFilterFollowsProvisionalThenCollectedTags(t *testing.T) {
	db, s := openCatalog(t)
	projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 1, Token: "token1"}, Title: "Feed", Tags: []string{"female:big hat"}}, time.Now().UTC())
	retain(t, db, panda.Metadata{ID: 2, Posted: 2}, true)
	setOverride(t, s, collectorapi.ScoreKindTag, "female:big hat", collectorapi.ScoreRuleAlwaysHide)
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), []int64{2})
	assertCatalogIDs(t, scorePage(t, s, collectorapi.ScoreFilterHidden, collectorapi.CatalogOptions{}), []int64{1})
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:glasses"}}, true)
	assertCatalogIDs(t, scorePage(t, s, "", collectorapi.CatalogOptions{}), []int64{2, 1})
}
