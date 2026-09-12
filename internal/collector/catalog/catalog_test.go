package catalog

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func openCatalog(t *testing.T) (*sql.DB, *Service) {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, New(db, "https://favorites.example.test")
}

func retain(t *testing.T, db *sql.DB, entry panda.Metadata, project bool) {
	t.Helper()
	entry.Token = fmt.Sprintf("token%d", entry.ID)
	body, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT DO NOTHING`, entry.ID, entry.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (?, ?, 0)
		ON CONFLICT (gallery_id) DO UPDATE SET body = excluded.body`, entry.ID, body); err != nil {
		t.Fatal(err)
	}
	if project {
		if err := Project(t.Context(), tx, entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogSearchOrderingAndPagination(t *testing.T) {
	db, s := openCatalog(t)
	entries := []panda.Metadata{
		{ID: 99, Title: "Old high ID", Posted: 100, Tags: []string{"other:strawberry", "parody:show-z"}},
		{ID: 1, Title: "Berry Blue ÉTÉ", TitleJapanese: "夏物語", Posted: 300, FileCount: 12,
			ThumbnailURL: "https://images.example.test/cover.jpg", Tags: []string{"other:berry blue", "other:red", "other:green", "artist:artist y"}},
		{ID: 2, TitleJapanese: "日本語のみ", Posted: 300, Tags: []string{"other:red", "other:blue", "other:yellow", "artist:artist y extended"}},
		{ID: 3, Title: "Hidden", Posted: 400, Expunged: true, Tags: []string{"other:green"}},
	}
	for _, entry := range entries {
		retain(t, db, entry, false)
	}
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (500, 'pending')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Backfill(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		want  []int64
	}{
		{"", []int64{2, 1, 99}},
		{"title:夏物語", []int64{1}},
		{"title:été", []int64{1}},
		{"tag:berry", []int64{1}},
		{"tag:straw", []int64{99}},
		{"red ~blue ~green -yellow", []int64{1}},
		{`a:"artist y$"`, []int64{1}},
		{`-a:artist_y$`, []int64{2, 99}},
		{"p:show-z", []int64{99}},
		{"parody:show-z", []int64{99}},
		{"-red", []int64{99}},
		{"missing", []int64{}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			result, err := s.List(t.Context(), collectorapi.CatalogOptions{Query: tc.query, Page: 1, PageSize: 100})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]int64, 0, len(result.Items))
			for _, item := range result.Items {
				got = append(got, item.GalleryID)
			}
			if !reflect.DeepEqual(got, tc.want) || result.Total != int64(len(tc.want)) {
				t.Fatalf("got %v total %d, want %v", got, result.Total, tc.want)
			}
		})
	}
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{Query: "red", Page: 999, PageSize: 1})
	if err != nil || result.Total != 2 || result.Page != 2 || result.TotalPages != 2 || len(result.Items) != 1 {
		t.Fatalf("filtered page: %+v, %v", result, err)
	}
	item := result.Items[0]
	if item.PageCount != 12 || item.PostedAt.Unix() != 300 || item.URL != "https://favorites.example.test/g/1/token1/" || item.ThumbnailURL != entries[1].ThumbnailURL {
		t.Fatalf("browse fields: %+v", item)
	}
	result, err = s.List(t.Context(), collectorapi.CatalogOptions{Page: 1, PageSize: 100, IncludeExpunged: true})
	if err != nil || result.Total != 4 || result.Items[0].GalleryID != 3 || result.Items[1].Title != "日本語のみ" {
		t.Fatalf("expunged/title fallback: %+v, %v", result, err)
	}
}

func TestCatalogProjectionUpdatesAndRawTagCompletions(t *testing.T) {
	db, s := openCatalog(t)
	raw := `Odd: comma, "quote" \slash $dollar`
	entry := panda.Metadata{ID: 1, Title: "Original", Posted: 10, Tags: []string{"artist:" + raw, "other:under_score", "other:旧作"}}
	retain(t, db, entry, true)
	for _, query := range []string{"a:odd", "tag:under", "旧"} {
		completion, err := s.Complete(t.Context(), "title:missing ~"+query, len("title:missing ~")+len([]rune(query)))
		if err != nil || len(completion.Items) != 1 {
			t.Fatalf("%q completions: %+v, %v", query, completion, err)
		}
		result, err := s.List(t.Context(), collectorapi.CatalogOptions{Query: completion.Items[0].Term, Page: 1, PageSize: 24})
		if err != nil || result.Total != 1 {
			t.Fatalf("raw completion query %q: %+v, %v", completion.Items[0].Term, result, err)
		}
	}
	entry.Title, entry.Tags, entry.Expunged = "Updated", []string{"other:new"}, true
	retain(t, db, entry, true)
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{Page: 1, PageSize: 24})
	if err != nil || result.Total != 0 {
		t.Fatalf("updated availability: %+v, %v", result, err)
	}
	result, err = s.List(t.Context(), collectorapi.CatalogOptions{Page: 1, PageSize: 24, IncludeExpunged: true})
	if err != nil || result.Total != 1 || result.Items[0].Title != "Updated" {
		t.Fatalf("updated projection: %+v, %v", result, err)
	}
	completion, err := s.Complete(t.Context(), "a:odd", 5)
	if err != nil || len(completion.Items) != 1 || completion.Items[0].Value != raw {
		t.Fatalf("retained raw vocabulary: %+v, %v", completion, err)
	}
	result, err = s.List(t.Context(), collectorapi.CatalogOptions{Query: completion.Items[0].Term, Page: 1, PageSize: 24, IncludeExpunged: true})
	if err != nil || result.Total != 0 {
		t.Fatalf("obsolete assignments retained: %+v, %v", result, err)
	}
}

func TestCatalogDecodesTitlesForDisplayAndSearch(t *testing.T) {
	db, s := openCatalog(t)
	entries := []panda.Metadata{
		{ID: 1, Title: ` Rock &amp; Roll &quot;ÉTÉ&quot; `, TitleJapanese: " 夏 &amp; 冬 "},
		{ID: 2, Title: "&nbsp;", TitleJapanese: " 日本語 &amp; 続編 "},
	}
	for i, entry := range entries {
		retain(t, db, entry, i == 1)
	}
	if err := s.Backfill(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		query string
		title string
	}{
		{`title:"rock & roll"`, `Rock & Roll "ÉTÉ"`},
		{`title:été`, `Rock & Roll "ÉTÉ"`},
		{`title:"夏 & 冬"`, `Rock & Roll "ÉTÉ"`},
		{`title:"日本語 & 続編"`, "日本語 & 続編"},
	} {
		result, err := s.List(t.Context(), collectorapi.CatalogOptions{Query: tc.query, Page: 1, PageSize: 24})
		if err != nil || result.Total != 1 || result.Items[0].Title != tc.title {
			t.Errorf("%s: %+v, %v; want %q", tc.query, result, err, tc.title)
		}
	}
	for _, entry := range entries {
		var body []byte
		if err := db.QueryRow("SELECT body FROM gallery_metadata WHERE gallery_id = ?", entry.ID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		var retained panda.Metadata
		if err := json.Unmarshal(body, &retained); err != nil || retained.Title != entry.Title || retained.TitleJapanese != entry.TitleJapanese {
			t.Fatalf("raw titles changed: %+v, %v", retained, err)
		}
	}
}

func TestBackfillBatchesAndResumes(t *testing.T) {
	db, s := openCatalog(t)
	for id := int64(1); id <= 101; id++ {
		retain(t, db, panda.Metadata{ID: id, Posted: id}, false)
	}
	if count, _, err := s.backfillBatch(t.Context(), 0); err != nil || count != 100 {
		t.Fatalf("first batch = %d, %v", count, err)
	}
	s = New(db, "https://favorites.example.test")
	for range 2 {
		if err := s.Backfill(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{Page: 1, PageSize: 100})
	if err != nil || result.Total != 101 || len(result.Items) != 100 || result.Items[0].GalleryID != 101 {
		t.Fatalf("resumed backfill: %+v, %v", result, err)
	}
	for _, options := range []collectorapi.CatalogOptions{{Page: 0, PageSize: 24}, {Page: 1, PageSize: 0}, {Page: 1, PageSize: 101}} {
		if _, err := s.List(t.Context(), options); !errors.Is(err, ErrInvalidPagination) {
			t.Fatalf("invalid pagination: %v", err)
		}
	}
	if _, err := s.List(t.Context(), collectorapi.CatalogOptions{Query: `"unfinished`, Page: 1, PageSize: 24}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid query: %v", err)
	}
}
