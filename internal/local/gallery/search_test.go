package gallery

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/fuzzy-moose/tana/internal/local/tag"
)

func searchGallery(t *testing.T, r *SQLiteRepository, title string, values ...tag.Value) Gallery {
	t.Helper()
	g, err := r.Create(t.Context(), title, nil)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := r.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := tag.NewSQLiteRepository(r.db).ReplaceForGalleryTx(t.Context(), tx, g.ID, values); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestSearchMatchingAndBooleanLogic(t *testing.T) {
	_, _, _, r := openRepositories(t)
	fixtures := []struct {
		title string
		tags  []tag.Value
	}{
		{"Berry Blue 2010", []tag.Value{{Namespace: "artist", Value: "artist y"}, {Namespace: "other", Value: "berry blue"}, {Namespace: "other", Value: "red"}, {Namespace: "other", Value: "green"}}},
		{"Strawberry 2011", []tag.Value{{Namespace: "artist", Value: "artist y extended"}, {Namespace: "other", Value: "strawberry"}, {Namespace: "other", Value: "red"}, {Namespace: "other", Value: "blue"}, {Namespace: "other", Value: "yellow"}}},
		{"Story Arc", []tag.Value{{Namespace: "artist", Value: "artist y"}, {Namespace: "character", Value: "char x"}, {Namespace: "other", Value: "blue"}, {Namespace: "other", Value: "red"}}},
		{"Other", []tag.Value{{Namespace: "series", Value: "story arc"}, {Namespace: "parody", Value: "show-z"}, {Namespace: "other", Value: "topic-a extended"}, {Namespace: "artist", Value: "berry"}, {Namespace: "custom", Value: "custom one"}}},
		{`A blue box, literal ~hello "quote" C:\shelf 100% ÉTÉ 🦊`, []tag.Value{{Namespace: "other", Value: "ai"}, {Namespace: "other", Value: "x-berry.blue"}, {Namespace: "other", Value: "topic-a"}}},
	}
	for _, fixture := range fixtures {
		searchGallery(t, r, fixture.title, fixture.tags...)
	}
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"", []int{0, 1, 2, 3, 4}},
		{" , , ", []int{0, 1, 2, 3, 4}},
		{"berry", []int{0, 1, 3, 4}},
		{"tag:berry", []int{0, 3, 4}},
		{"title:berry", []int{0, 1}},
		{"berry$", []int{3}},
		{"tag:blue$", []int{1, 2}},
		{"topic-a", []int{3, 4}},
		{"topic-a$", []int{4}},
		{`a:"artist y"`, []int{0, 1, 2}},
		{`ARTIST:artist_y$`, []int{0, 2}},
		{`a:"artist y$"`, []int{0, 2}},
		{`"artist y$"`, []int{0, 2}},
		{`-a:"artist y$"`, []int{1, 3, 4}},
		{`~a:"artist y$" ~tag:yellow$`, []int{0, 1, 2}},
		{`a:artist_y$ c:char_x`, []int{2}},
		{`custom:custom`, []int{3}},
		{`p:show-z`, []int{3}},
		{`parody:show-z`, []int{3}},
		{`p:story_arc`, nil},
		{`title:"story arc"`, []int{2}},
		{`"story arc"`, []int{2, 3}},
		{`title:story_arc`, []int{2}},
		{"red,blue", []int{0, 1, 2}},
		{"red ~blue ~green -yellow", []int{0, 2}},
		{"~blue -yellow red ~green", []int{0, 2}},
		{"~green ~yellow", []int{0, 1}},
		{"~green", []int{0}},
		{"-red", []int{3, 4}},
		{"-a:artist_y$", []int{1, 3, 4}},
		{"-title:2010 -2011", []int{2, 3, 4}},
		{`title:"box, literal"`, []int{4}},
		{`title:"~hello"`, []int{4}},
		{`title:"\"quote\""`, []int{4}},
		{`title:"C:\\shelf"`, []int{4}},
		{"%", []int{4}},
		{"a blue box", []int{4}},
		{"ai", []int{4}},
		{"title:ÉTÉ", []int{4}},
		{"missing", nil},
	} {
		t.Run(tc.query, func(t *testing.T) {
			listing, err := r.Browse(t.Context(), tc.query, 1, 100)
			if err != nil {
				t.Fatal(err)
			}
			var want, got []string
			for _, index := range tc.want {
				want = append(want, fixtures[index].title)
			}
			for _, item := range listing.Items {
				got = append(got, item.Title)
			}
			slices.Sort(want)
			slices.Sort(got)
			if !reflect.DeepEqual(got, want) || listing.Total != int64(len(want)) {
				t.Fatalf("got %v (total %d), want %v", got, listing.Total, want)
			}
		})
	}
	listing, err := r.Browse(t.Context(), "a:artist_y", 999, 1)
	if err != nil || listing.Total != 3 || listing.Page != 3 || len(listing.Items) != 1 {
		t.Fatalf("filtered pagination: %+v, %v", listing, err)
	}
}

func TestSearchNamespacePrecedenceAndShortForms(t *testing.T) {
	_, _, _, r := openRepositories(t)
	for short, name := range namespaceShortForms {
		searchGallery(t, r, name, tag.Value{Namespace: name, Value: "example"})
		for _, query := range []string{name + ":example", strings.ToUpper(short) + ":EXAMPLE"} {
			listing, err := r.Browse(t.Context(), query, 1, 24)
			if err != nil || listing.Total != 1 || listing.Items[0].Title != name {
				t.Fatalf("%s: %+v, %v", query, listing, err)
			}
		}
	}
	searchGallery(t, r, "custom short name", tag.Value{Namespace: "a", Value: "example"})
	listing, err := r.Browse(t.Context(), "a:example", 1, 24)
	if err != nil || listing.Total != 1 || listing.Items[0].Title != "custom short name" {
		t.Fatalf("full name must win: %+v, %v", listing, err)
	}
}

func TestInvalidSearchQueries(t *testing.T) {
	_, _, _, r := openRepositories(t)
	for _, query := range []string{`"unclosed`, `artist:`, `unknown:value`, `title:blue$`, `title:"blue$"`, `a:"artist y"$`, `"$"`, `-~blue`, `~-blue`, `--blue`, `-`, `~`, `""`, `$`, `tag:blue$$`, `"blue"tail`, `a:"bad\q"`, `a:b:c`} {
		if _, err := r.Browse(t.Context(), query, 1, 24); !errors.Is(err, ErrInvalidQuery) {
			t.Errorf("%q: want invalid query, got %v", query, err)
		}
	}
}

func TestCompletionUsesCatalogRankingAndActiveToken(t *testing.T) {
	_, _, _, r := openRepositories(t)
	g := searchGallery(t, r, "deleted gallery",
		tag.Value{Namespace: "artist", Value: "artist y"}, tag.Value{Namespace: "artist", Value: "artist y extended"},
		tag.Value{Namespace: "artist", Value: "the artist y"}, tag.Value{Namespace: "other", Value: "artist y"},
		tag.Value{Namespace: "other", Value: "strawberry"}, tag.Value{Namespace: "other", Value: "red-berry"}, tag.Value{Namespace: "other", Value: "blue.berry"})
	if err := r.Delete(t.Context(), g.ID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		at    string
		want  []string
	}{
		{`artist_y`, `artist_y`, []string{`artist:"artist y$"`, `other:"artist y$"`, `artist:"artist y extended$"`, `artist:"the artist y$"`}},
		{`-a:"artist y`, `-a:"artist y`, []string{`-artist:"artist y$"`, `-artist:"artist y extended$"`, `-artist:"the artist y$"`}},
		{`-a:"artist y$"`, `-a:"artist y$`, []string{`-artist:"artist y$"`, `-artist:"artist y extended$"`, `-artist:"the artist y$"`}},
		{`title:missing ~a:art -red`, `title:missing ~a:art`, []string{`~artist:"artist y$"`, `~artist:"artist y extended$"`, `~artist:"the artist y$"`}},
		{`title:"🦊" ~a:artist_y$ -red`, `title:"🦊" ~a:artist`, []string{`~artist:"artist y$"`, `~artist:"artist y extended$"`, `~artist:"the artist y$"`}},
		{`berry`, `berry`, []string{`other:blue.berry$`, `other:red-berry$`}},
		{`a:`, `a:`, nil},
		{``, ``, nil},
		{`title:art`, `title:art`, nil},
		{`unknown:art`, `unknown:art`, nil},
		{`blue `, `blue `, nil},
	} {
		t.Run(tc.query+tc.at, func(t *testing.T) {
			result, err := r.Complete(t.Context(), tc.query, len(utf16.Encode([]rune(tc.at))))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, item := range result.Items {
				got = append(got, item.Term)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if len(got) > 0 {
				units := utf16.Encode([]rune(tc.query))
				replaced := string(utf16.Decode(units[:result.Start])) + got[0] + string(utf16.Decode(units[result.End:]))
				if _, err := r.Browse(t.Context(), replaced, 1, 24); err != nil {
					t.Fatalf("completion did not produce a valid query %q: %v", replaced, err)
				}
				if strings.HasSuffix(tc.query, " -red") && !strings.HasSuffix(replaced, " -red") {
					t.Fatalf("completion removed following term: %s", replaced)
				}
			}
		})
	}
	var values []tag.Value
	for i := range 15 {
		values = append(values, tag.Value{Namespace: "other", Value: fmt.Sprintf("many %02d", i)})
	}
	searchGallery(t, r, "many", values...)
	result, err := r.Complete(t.Context(), "many", 4)
	if err != nil || len(result.Items) != 10 || result.Items[9].Value != "many 09" {
		t.Fatalf("completion limit: %+v, %v", result, err)
	}
}
