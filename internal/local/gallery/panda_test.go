package gallery

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/source"
)

func TestPandaBrowseFiltersBeforePaginationAndKeepsMissingFavoriteTimesLast(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	newer, older := int64(200), int64(100)
	var facts []PandaFact
	var expectedCandidates []PandaCandidate
	var firstFile int64
	for i, item := range []struct {
		name, category string
		pandaID        int64
		favoritedAt    *int64
	}{
		{"Book A [11].cbz", "manga", 11, &newer},
		{"Book B [22].cbz", "doujinshi", 22, &older},
		{"Book C [11].cbz", "manga", 11, &newer},
		{"Book D [33].cbz", "", 33, nil},
		{"Book E.cbz", "", 0, nil},
	} {
		_, s, files := inventory(t, libraries, sources, item.name, source.Archive, "1.jpg")
		if _, err := galleries.CreateFromSource(t.Context(), s.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstFile = files["1.jpg"]
		}
		if item.pandaID != 0 {
			expectedCandidates = append(expectedCandidates, PandaCandidate{SourceID: s.ID, PandaID: item.pandaID})
			facts = append(facts, PandaFact{SourceID: s.ID, Category: item.category, FavoritedAt: item.favoritedAt})
		}
	}
	// A source rooted directly in its library uses the library basename.
	l, err := libraries.Create(t.Context(), "Root", filepath.Join(t.TempDir(), "Book Root [44]"))
	if err != nil {
		t.Fatal(err)
	}
	s, err := sources.Create(t.Context(), l.ID, ".", source.Directory, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := galleries.CreateFromSource(t.Context(), s.ID); err != nil {
		t.Fatal(err)
	}
	expectedCandidates = append(expectedCandidates, PandaCandidate{SourceID: s.ID, PandaID: 44})
	facts = append(facts, PandaFact{SourceID: s.ID, Category: "artist cg"})
	// Reusing a Panda source's pages and name does not link a manual gallery.
	if _, err := galleries.Create(t.Context(), "Book Manual [11]", []int64{firstFile}); err != nil {
		t.Fatal(err)
	}
	candidates, err := galleries.PandaCandidates(t.Context())
	if err != nil || !reflect.DeepEqual(candidates, expectedCandidates) {
		t.Fatalf("candidates: %+v, %v; want %+v", candidates, err, expectedCandidates)
	}

	for _, tc := range []struct {
		name       string
		search     string
		categories []string
		sort       BrowseSort
		page, size int64
		total      int64
		titles     []string
	}{
		{"category union", "", []string{"Manga", "doujinshi", "manga"}, SortTitle, 2, 2, 3, []string{"Book C [11]"}},
		{"category and search", `title:"Book B"`, []string{"manga", "doujinshi"}, SortTitle, 1, 2, 1, []string{"Book B [22]"}},
		{"category without favorite", "", []string{"artist cg"}, SortFavoritedAsc, 1, 2, 1, []string{"Book Root [44]"}},
		{"ascending first", "", nil, SortFavoritedAsc, 1, 2, 7, []string{"Book B [22]", "Book A [11]"}},
		{"ascending boundary", "", nil, SortFavoritedAsc, 2, 2, 7, []string{"Book C [11]", "Book D [33]"}},
		{"descending first", "", nil, SortFavoritedDesc, 1, 2, 7, []string{"Book A [11]", "Book C [11]"}},
		{"descending boundary", "", nil, SortFavoritedDesc, 2, 2, 7, []string{"Book B [22]", "Book D [33]"}},
		{"missing ascending last", "", nil, SortFavoritedAsc, 3, 2, 7, []string{"Book E", "Book Manual [11]"}},
		{"missing descending last", "", nil, SortFavoritedDesc, 3, 2, 7, []string{"Book E", "Book Manual [11]"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listing, err := galleries.BrowseFiltered(t.Context(), tc.search, tc.page, tc.size, BrowseOptions{
				Categories: tc.categories, Sort: tc.sort, PandaFacts: facts,
			})
			if err != nil {
				t.Fatal(err)
			}
			var titles []string
			for _, item := range listing.Items {
				titles = append(titles, item.Title)
				if item.PageCount != 1 {
					t.Fatalf("page count: %+v", item)
				}
			}
			if listing.Total != tc.total || listing.Page != tc.page || !reflect.DeepEqual(titles, tc.titles) {
				t.Fatalf("listing: %+v; want total %d page %d titles %v", listing, tc.total, tc.page, tc.titles)
			}
		})
	}
}
