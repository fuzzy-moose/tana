package catalog

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// settleScope runs the scope count work until none is left.
func settleScope(t *testing.T, s *Service) {
	t.Helper()
	for range 10000 {
		more, err := s.UpdateScopeCounts(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
	t.Fatal("scope count work does not end")
}

// scopeCounts reads N and every stored count of the catalog in scope, keyed
// by kind and label.
func scopeCounts(t *testing.T, db *pgxpool.Pool) (int, map[string]int) {
	t.Helper()
	var galleries int
	if err := db.QueryRow(t.Context(), "SELECT galleries FROM catalog_scope").Scan(&galleries); err != nil {
		t.Fatal(err)
	}
	return galleries, labelledCounts(t, db, `SELECT c.kind, COALESCE(t.namespace || ':' || t.value, c.value), c.galleries
		FROM catalog_scope_counts c LEFT JOIN catalog_tags t ON c.kind = 'tag' AND t.id = c.tag_id`)
}

// recount counts the catalog in scope of the query from the catalog itself.
func recount(t *testing.T, db *pgxpool.Pool, query string) (int, map[string]int) {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	scope, args, _, err := catalogPredicate(t.Context(), tx, collectorapi.CatalogOptions{DefaultQuery: query, IncludeExpunged: true})
	if err != nil {
		t.Fatal(err)
	}
	var galleries int
	if err := tx.QueryRow(t.Context(), "SELECT count(*) FROM catalog_galleries g WHERE "+scope, args...).Scan(&galleries); err != nil {
		t.Fatal(err)
	}
	return galleries, labelledCounts(t, tx, `SELECT 'tag', t.namespace || ':' || t.value, count(*)
		FROM catalog_galleries g JOIN catalog_gallery_tags gt ON gt.gallery_id = g.gallery_id JOIN catalog_tags t ON t.id = gt.tag_id
		WHERE `+scope+` GROUP BY t.id
		UNION ALL SELECT 'uploader', g.uploader, count(*) FROM catalog_galleries g WHERE g.uploader <> '' AND (`+scope+`) GROUP BY g.uploader
		UNION ALL SELECT 'category', g.category, count(*) FROM catalog_galleries g WHERE g.category <> '' AND (`+scope+`) GROUP BY g.category`, args...)
}

// labelledCounts reads rows of kind, label and count.
func labelledCounts(t *testing.T, db interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, query string, args ...any) map[string]int {
	t.Helper()
	rows, err := db.Query(t.Context(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var kind, label string
		var count int
		if err := rows.Scan(&kind, &label, &count); err != nil {
			t.Fatal(err)
		}
		counts[kind+" "+label] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return counts
}

// assertScopeExact settles the scope counts and compares them with a count
// of the catalog itself.
func assertScopeExact(t *testing.T, db *pgxpool.Pool, s *Service, query string) {
	t.Helper()
	settleScope(t, s)
	galleries, counts := scopeCounts(t, db)
	wantGalleries, wantCounts := recount(t, db, query)
	if galleries != wantGalleries || !maps.Equal(counts, wantCounts) {
		t.Fatalf("scope counts = %d %v, want %d %v", galleries, counts, wantGalleries, wantCounts)
	}
}

func TestCalculationWaitsForTheScopeCounts(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 6, panda.Metadata{Category: "Manga", Tags: []string{"other:a"}})
	favor(t, db, "42", 0, 1, 2, 3)

	preview := previewRaw(t, s, collectorapi.ScorePreviewOptions{})
	if preview.Candidate != nil || preview.NoModel != collectorapi.ScoreCountsPreparing || preview.CountsProgress == nil {
		t.Fatalf("preview before the counts = candidate %+v, no model %q, progress %v", preview.Candidate, preview.NoModel, preview.CountsProgress)
	}
	if preview.Evidence.Favorites != 3 {
		t.Fatalf("favorites in scope = %d, want 3", preview.Evidence.Favorites)
	}

	settleScope(t, s)
	preview = previewRaw(t, s, collectorapi.ScorePreviewOptions{})
	if preview.Candidate == nil || preview.NoModel != "" || preview.CountsProgress != nil {
		t.Fatalf("preview after the counts = candidate %+v, no model %q", preview.Candidate, preview.NoModel)
	}
	var catalog int
	if err := db.QueryRow(t.Context(), "SELECT catalog_in_scope FROM panda_score_models WHERE id = $1", preview.Candidate.ID).Scan(&catalog); err != nil {
		t.Fatal(err)
	}
	if catalog != 6 {
		t.Fatalf("catalog in scope = %d, want 6", catalog)
	}
}

func TestScopeCountsBuildInBatches(t *testing.T) {
	db, s := openCatalog(t)
	s.scopeBatch = 2
	retainRange(t, db, 1, 3, panda.Metadata{Uploader: "Up", Category: "Manga", Tags: []string{"other:a", "language:english"}})
	retainRange(t, db, 4, 7, panda.Metadata{Category: "Doujinshi", Tags: []string{"other:a", "other:b"}})
	retain(t, db, panda.Metadata{ID: 8, Posted: 8, Expunged: true, Tags: []string{"other:b"}}, true)
	previewRaw(t, s, collectorapi.ScorePreviewOptions{DefaultQuery: "-l:english$"})

	more, err := s.UpdateScopeCounts(t.Context())
	if err != nil || !more {
		t.Fatalf("first batch: more %v, error %v", more, err)
	}
	var ready bool
	if err := db.QueryRow(t.Context(), "SELECT ready FROM catalog_scope").Scan(&ready); err != nil || ready {
		t.Fatalf("ready after one batch = %v, %v", ready, err)
	}
	assertScopeExact(t, db, s, "-l:english$")
}

func TestCatalogWritesKeepTheScopeCountsExact(t *testing.T) {
	db, s := openCatalog(t)
	query := "-l:english$"
	retainRange(t, db, 1, 4, panda.Metadata{Uploader: "Up", Category: "Manga", Tags: []string{"other:a"}})
	retainRange(t, db, 5, 6, panda.Metadata{Category: "Manga", Tags: []string{"other:a", "language:english"}})
	previewRaw(t, s, collectorapi.ScorePreviewOptions{DefaultQuery: query})
	assertScopeExact(t, db, s, query)

	// A new gallery, a tag change into and out of the scope, a new uploader
	// and category, and a gallery removed with its tags.
	retain(t, db, panda.Metadata{ID: 7, Posted: 7, Uploader: "Other", Category: "Doujinshi", Tags: []string{"other:new"}}, true)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Uploader: "Up", Category: "Manga", Tags: []string{"other:a", "language:english"}}, true)
	retain(t, db, panda.Metadata{ID: 5, Posted: 5, Uploader: "Up", Category: "Western", Tags: []string{"other:b"}}, true)
	if _, err := db.Exec(t.Context(), "UPDATE catalog_galleries SET uploader = 'Renamed', category = 'misc' WHERE gallery_id = 2"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM catalog_galleries WHERE gallery_id = 3"); err != nil {
		t.Fatal(err)
	}
	// Tag rows alone: gallery 4 loses a tag, and gallery 2 leaves the scope.
	if _, err := db.Exec(t.Context(), `DELETE FROM catalog_gallery_tags gt USING catalog_tags t
		WHERE t.id = gt.tag_id AND t.value = 'a' AND gt.gallery_id = 4`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO catalog_gallery_tags (gallery_id, tag_id)
		SELECT 2, id FROM catalog_tags WHERE namespace = 'language' AND value = 'english'`); err != nil {
		t.Fatal(err)
	}
	assertScopeExact(t, db, s, query)
}

func TestADefaultFilterChangeRebuildsTheScopeCounts(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Category: "Manga", Tags: []string{"other:a"}})
	retainRange(t, db, 4, 6, panda.Metadata{Category: "Doujinshi", Tags: []string{"other:a", "other:b"}})
	favor(t, db, "42", 0, 1, 2, 3, 4, 5, 6)
	previewWith(t, s, collectorapi.ScorePreviewOptions{})
	assertScopeExact(t, db, s, "")

	options := collectorapi.ScorePreviewOptions{DefaultQuery: "-other:b"}
	if preview := previewRaw(t, s, options); preview.NoModel != collectorapi.ScoreCountsPreparing {
		t.Fatalf("no model after a filter change = %q", preview.NoModel)
	}
	assertScopeExact(t, db, s, "-other:b")
	if preview := previewWith(t, s, options); preview.Candidate == nil {
		t.Fatalf("no candidate after the rebuild: %q", preview.NoModel)
	}
}

func TestAMeasurementNeedsNoScopeCounts(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	candidate := previewScore(t, s).Candidate
	// A filter change starts a new build; the stored candidate still measures.
	preview := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultQuery: "-other:nothing", CandidateID: candidate.ID})
	assertSameCandidate(t, preview.Candidate, candidate)
}
