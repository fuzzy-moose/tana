package gallery

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/source"
)

func TestDetailPandaCandidateUsesLinkedSource(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	l, err := libraries.Create(t.Context(), "Library", filepath.Join(t.TempDir(), "Root [44]"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		kind source.Kind
		want int64
	}{
		{"Book [11].cbz", source.Archive, 11},
		{"Book [22]", source.Directory, 22},
		{".", source.Directory, 44},
		{"Book", source.Directory, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := sources.Create(t.Context(), l.ID, tc.name, tc.kind, []string{"1.jpg"})
			if err != nil {
				t.Fatal(err)
			}
			g, err := galleries.CreateFromSource(t.Context(), s.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := galleries.Rename(t.Context(), g.ID, "Edited title [99]"); err != nil {
				t.Fatal(err)
			}
			detail, err := galleries.Detail(t.Context(), g.ID)
			if err != nil || detail.PandaCandidateID != tc.want {
				t.Fatalf("detail candidate: %d, %v; want %d", detail.PandaCandidateID, err, tc.want)
			}
			pages, err := galleries.Pages(t.Context(), g.ID)
			if err != nil {
				t.Fatal(err)
			}
			manual, err := galleries.Create(t.Context(), "Manual [11]", []int64{pages[0].SourceFileID})
			if err != nil {
				t.Fatal(err)
			}
			detail, err = galleries.Detail(t.Context(), manual.ID)
			if err != nil || detail.PandaCandidateID != 0 {
				t.Fatalf("manual gallery candidate: %d, %v", detail.PandaCandidateID, err)
			}
		})
	}
}

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
	candidates, err := galleries.PandaCandidates(t.Context(), "")
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

func TestBrowsePaginationBreaksTitleAndFavoriteTiesByID(t *testing.T) {
	_, libraries, sources, galleries := openRepositories(t)
	zero, newer := int64(0), int64(100)
	var ids []int64
	var facts []PandaFact
	counts := map[int64]int64{}
	for i, at := range []*int64{nil, &zero, &zero, &newer} {
		pages := []string{"1.jpg"}
		if i%2 != 0 {
			pages = append(pages, "2.jpg")
		}
		_, s, _ := inventory(t, libraries, sources, fmt.Sprintf("Book [%d]", i+1), source.Directory, pages...)
		g, err := galleries.CreateFromSource(t.Context(), s.ID)
		if err != nil {
			t.Fatal(err)
		}
		title := "Same"
		if i%2 != 0 {
			title = "same"
		}
		if _, err := galleries.Rename(t.Context(), g.ID, title); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, g.ID)
		counts[g.ID] = int64(len(pages))
		facts = append(facts, PandaFact{SourceID: s.ID, FavoritedAt: at})
	}
	empty, err := galleries.Create(t.Context(), "Same", nil)
	if err != nil {
		t.Fatal(err)
	}
	ids = append(ids, empty.ID)
	for _, tc := range []struct {
		sort BrowseSort
		want []int64
	}{
		{SortTitle, ids},
		{SortFavoritedAsc, []int64{ids[1], ids[2], ids[3], ids[0], ids[4]}},
		{SortFavoritedDesc, []int64{ids[3], ids[1], ids[2], ids[0], ids[4]}},
	} {
		t.Run(string(tc.sort), func(t *testing.T) {
			var got []int64
			for page := int64(1); page <= 3; page++ {
				listing, err := galleries.BrowseFiltered(t.Context(), "", page, 2, BrowseOptions{Sort: tc.sort, PandaFacts: facts})
				if err != nil || listing.Total != 5 || listing.Page != page {
					t.Fatalf("page %d: %+v, %v", page, listing, err)
				}
				for _, item := range listing.Items {
					got = append(got, item.ID)
					if item.PageCount != counts[item.ID] {
						t.Fatalf("page count: %+v, want %d", item, counts[item.ID])
					}
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("page order: %v, want %v", got, tc.want)
			}
		})
	}
}
