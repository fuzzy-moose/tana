package gallery

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func BenchmarkBrowse(b *testing.B) {
	db := pgtest.Open(b, storage.Migrations())
	_, err := db.Exec(b.Context(), `
		INSERT INTO libraries(id, name, path) VALUES (1, 'Library', '/library');
		INSERT INTO sources(id, library_id, path, kind)
		SELECT id, 1, format('Book [%s]', id), 'directory' FROM generate_series(1, 10000) ids(id);
		INSERT INTO source_files(id, source_id, path) SELECT id, id, '1.jpg' FROM sources;
		INSERT INTO galleries(id, title, source_id) SELECT id, 'Book ' || lpad((10001 - id)::text, 5, '0'), id FROM sources;
		INSERT INTO gallery_pages SELECT g.id, p.position, g.id FROM galleries g CROSS JOIN generate_series(1, 100) p(position);
		INSERT INTO namespaces(id, name) VALUES (1, 'language'), (2, 'other');
		INSERT INTO tags(id, namespace_id, value) VALUES (1, 1, 'translated'), (2, 2, 'rare');
		INSERT INTO gallery_tags SELECT id, 1 FROM galleries WHERE id%3=0;
		INSERT INTO gallery_tags SELECT id, 2 FROM galleries WHERE id%1000=0;
		ANALYZE;
	`)
	if err != nil {
		b.Fatal(err)
	}
	facts := make([]PandaFact, 10000)
	for i := range facts {
		at := int64(i % 100)
		category := "manga"
		if i%2 == 0 {
			category = "doujinshi"
		}
		facts[i] = PandaFact{SourceID: int64(i + 1), Category: category, FavoritedAt: &at}
	}
	r := NewStore(db)
	for _, tc := range []struct {
		name, query string
		page        int64
		options     BrowseOptions
	}{
		{"title", "", 1, BrowseOptions{}},
		{"title_deep", "", 300, BrowseOptions{}},
		{"favorite", "", 1, BrowseOptions{Sort: SortFavoritedDesc, PandaFacts: facts}},
		{"favorite_deep", "", 300, BrowseOptions{Sort: SortFavoritedAsc, PandaFacts: facts}},
		{"category", "", 1, BrowseOptions{Categories: []string{"manga"}, PandaFacts: facts}},
		{"categories_favorite", "", 1, BrowseOptions{Categories: []string{"manga", "doujinshi"}, Sort: SortFavoritedDesc, PandaFacts: facts}},
		{"exclude", "-language:translated", 1, BrowseOptions{}},
		{"exact_tag", "other:rare$", 1, BrowseOptions{}},
		{"prefix_tag", "tag:rar", 1, BrowseOptions{}},
		{"title_search", "title:123", 1, BrowseOptions{}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				if _, err := r.BrowseFiltered(b.Context(), tc.query, tc.page, 24, tc.options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
