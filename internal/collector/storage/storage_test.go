package storage_test

import (
	"io/fs"
	"maps"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestMigrationsCreateBaselineWithSingletons(t *testing.T) {
	files, err := fs.Glob(storage.Migrations(), "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	pool := pgtest.Open(t, storage.Migrations())
	var versions []int32
	rows, err := pool.Query(t.Context(), "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var v int32
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(versions) != len(files) {
		t.Fatalf("schema versions = %v, want 1..%d", versions, len(files))
	}
	for i, v := range versions {
		if v != int32(i+1) {
			t.Fatalf("schema versions = %v, want 1..%d", versions, len(files))
		}
	}
	var bans, delayHours int
	var state string
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM panda_ban WHERE until_at IS NULL),
		(SELECT state FROM sitemap_run WHERE id = 1),
		(SELECT feed_metadata_delay_hours FROM metadata_collection_settings WHERE id = 1)`).Scan(&bans, &state, &delayHours); err != nil {
		t.Fatal(err)
	}
	if bans != 2 || state != "idle" || delayHours != 12 {
		t.Fatalf("seeded rows: bans=%d sitemap=%q delay=%d", bans, state, delayHours)
	}
}

func TestFavoriteAuthorsMigrationFillsFromStoredFavorites(t *testing.T) {
	all := storage.Migrations()
	upToEight := fstest.MapFS{}
	for _, name := range []string{"0001_baseline.sql", "0002_panda_gallery_displays.sql", "0003_panda_catalog_catch_up.sql",
		"0004_panda_gallery_verdicts.sql", "0005_panda_score_overrides.sql", "0006_panda_display_evidence.sql",
		"0007_panda_score_models.sql", "0008_panda_score_pins.sql"} {
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToEight[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToEight)
	// Gallery 2 is a favorite of two collections and counts once; gallery 4
	// has no catalog entry; artist:b and group:g carry fewer than two
	// favorites or another namespace.
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO favorite_categories (id, host, account_key, category, name) VALUES
			(1, 'https://panda.test', '42', 2, ''), (2, 'https://panda.test', '43', 3, '');
		INSERT INTO gallery_refs (gallery_id, token) SELECT g, 'token' FROM generate_series(1, 4) g;
		INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES
			(1, 1, 'token', now()), (1, 2, 'token', now()), (2, 2, 'token', now()), (2, 3, 'token', now()), (2, 4, 'token', now());
		INSERT INTO catalog_galleries (gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, posted, ordered_at)
			SELECT g, 'Gallery', 'gallery', '', '', now(), now() FROM generate_series(1, 3) g;
		INSERT INTO catalog_tags (id, namespace, value, value_lower) VALUES
			(1, 'artist', 'a', 'a'), (2, 'artist', 'b', 'b'), (3, 'group', 'g', 'g');
		INSERT INTO catalog_gallery_tags (gallery_id, tag_id) VALUES
			(1, 1), (2, 1), (3, 1), (1, 2), (1, 3), (2, 3);`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var tags, count int
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM favorite_authors),
		(SELECT favorites FROM favorite_authors WHERE tag_id = 1)`).Scan(&tags, &count); err != nil {
		t.Fatal(err)
	}
	if tags != 1 || count != 3 {
		t.Fatalf("favorite authors: rows=%d artist:a=%d", tags, count)
	}
}

func TestScoreModelsFromFavoritesMigrationDeletesEveryModel(t *testing.T) {
	all := storage.Migrations()
	upToEleven := fstest.MapFS{}
	for _, name := range []string{"0001_baseline.sql", "0002_panda_gallery_displays.sql", "0003_panda_catalog_catch_up.sql",
		"0004_panda_gallery_verdicts.sql", "0005_panda_score_overrides.sql", "0006_panda_display_evidence.sql",
		"0007_panda_score_models.sql", "0008_panda_score_pins.sql", "0009_favorite_authors.sql",
		"0010_panda_score_settings.sql", "0011_panda_score_models_without_favorites.sql"} {
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToEleven[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToEleven)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO catalog_tags (id, namespace, value, value_lower) VALUES (1, 'artist', 'a', 'a'), (2, 'male', 'hat', 'hat');
		INSERT INTO panda_score_models (id, active, calculated_at, threshold_tenths) OVERRIDING SYSTEM VALUE VALUES
			(1, true, now(), 5), (2, false, now(), NULL);
		INSERT INTO panda_score_weights (model_id, kind, tag_id, value, weight, interesting, not_for_me, neutral) VALUES
			(1, 'tag', 1, '', 0.5, 3, 0, 0), (2, 'uploader', NULL, 'Someone', -0.5, 0, 3, 0);
		INSERT INTO panda_score_pins (kind, tag_id, value, weight, created_at) VALUES ('tag', 1, '', -2, now()), ('uploader', NULL, 'Someone', 1, now());
		INSERT INTO panda_score_overrides (kind, tag_id, value, rule, created_at) VALUES ('tag', 2, '', 'never_hide', now());
		UPDATE panda_score_settings SET author_bonus = 1.5;`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var models, weights, pins, overrides int
	var bonus float64
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM panda_score_models),
		(SELECT count(*) FROM panda_score_weights),
		(SELECT count(*) FROM panda_score_pins),
		(SELECT count(*) FROM panda_score_overrides),
		(SELECT author_bonus FROM panda_score_settings)`).Scan(&models, &weights, &pins, &overrides, &bonus); err != nil {
		t.Fatal(err)
	}
	if models != 0 || weights != 0 || pins != 2 || overrides != 1 || bonus != 1.5 {
		t.Fatalf("models=%d weights=%d pins=%d overrides=%d bonus=%g", models, weights, pins, overrides, bonus)
	}

	service := catalog.New(pool, "https://panda.test")
	page := func() collectorapi.CatalogResult {
		t.Helper()
		result, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if filter := page().ScoreFilter; filter == nil || filter.ThresholdTenths != nil {
		t.Fatalf("score filter with an override: %+v", filter)
	}
	if _, err := pool.Exec(t.Context(), "DELETE FROM panda_score_overrides"); err != nil {
		t.Fatal(err)
	}
	if filter := page().ScoreFilter; filter != nil {
		t.Fatalf("score filter without overrides: %+v", filter)
	}
}

func TestFavoriteObservationsMigrationTimesEveryObservation(t *testing.T) {
	all := storage.Migrations()
	files, err := fs.Glob(all, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	upToTwelve := fstest.MapFS{}
	for _, name := range files {
		if name >= "0013" {
			continue
		}
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToTwelve[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToTwelve)
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'token1'), (2, 'token2');
		INSERT INTO favorite_observations (gallery_id) VALUES (1);`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var migrated bool
	if err := pool.QueryRow(t.Context(), "SELECT observed_at IS NOT NULL FROM favorite_observations WHERE gallery_id = 1").Scan(&migrated); err != nil || !migrated {
		t.Fatalf("migrated observation timed: %v, %v", migrated, err)
	}
	var observedAt, before time.Time
	if err := pool.QueryRow(t.Context(), "SELECT now()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), "INSERT INTO favorite_observations (gallery_id) VALUES (2) RETURNING observed_at").Scan(&observedAt); err != nil {
		t.Fatal(err)
	}
	var after time.Time
	if err := pool.QueryRow(t.Context(), "SELECT now()").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if observedAt.Before(before) || observedAt.After(after) {
		t.Fatalf("new observation at %v, want between %v and %v", observedAt, before, after)
	}
}

func TestDropVerdictsMigrationKeepsDisplays(t *testing.T) {
	all := storage.Migrations()
	files, err := fs.Glob(all, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	upToThirteen := fstest.MapFS{}
	for _, name := range files {
		if name >= "0014" {
			continue
		}
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToThirteen[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToThirteen)
	// Galleries 1 and 2 have verdicts with the displays recorded with them,
	// gallery 3 has only a display, and gallery 4 has neither.
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO gallery_refs (gallery_id, token) SELECT g, 'token' FROM generate_series(1, 4) g;
		INSERT INTO catalog_galleries (gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, posted, ordered_at)
			SELECT g, 'Gallery', 'gallery', '', '', now(), now() + g * interval '1 second' FROM generate_series(1, 4) g;
		INSERT INTO panda_gallery_verdicts (gallery_id, verdict, given_at) VALUES (1, 'not_for_me', now()), (2, 'interesting', now());
		INSERT INTO panda_gallery_displays (gallery_id, displayed_at) VALUES (1, now()), (2, now()), (3, now());`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var displays int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM panda_gallery_displays").Scan(&displays); err != nil || displays != 3 {
		t.Fatalf("displays = %d, %v", displays, err)
	}

	service := catalog.New(pool, "https://panda.test")
	list := func(unseen bool) collectorapi.CatalogResult {
		t.Helper()
		result, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, Unseen: unseen})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	listed := list(false)
	if listed.ScoreFilter != nil {
		t.Fatalf("score filter = %+v", listed.ScoreFilter)
	}
	displayed := map[int64]bool{}
	for _, item := range listed.Items {
		displayed[item.GalleryID] = item.Displayed
	}
	if want := map[int64]bool{1: true, 2: true, 3: true, 4: false}; !maps.Equal(displayed, want) {
		t.Fatalf("displayed = %v, want %v", displayed, want)
	}
	unseen := list(true)
	if len(unseen.Items) != 1 || unseen.Items[0].GalleryID != 4 {
		t.Fatalf("unseen = %+v", unseen.Items)
	}
}

func TestDropDisplayEvidenceMigrationKeepsDisplays(t *testing.T) {
	all := storage.Migrations()
	files, err := fs.Glob(all, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	upToFourteen := fstest.MapFS{}
	for _, name := range files {
		if name >= "0015" {
			continue
		}
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToFourteen[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToFourteen)
	// Gallery 1 has a display that was evidence, gallery 2 one that was not,
	// and gallery 3 none.
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO gallery_refs (gallery_id, token) SELECT g, 'token' FROM generate_series(1, 3) g;
		INSERT INTO catalog_galleries (gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, posted, ordered_at)
			SELECT g, 'Gallery', 'gallery', '', '', now(), now() + g * interval '1 second' FROM generate_series(1, 3) g;
		INSERT INTO panda_gallery_displays (gallery_id, displayed_at, evidence) VALUES (1, now(), true), (2, now(), false);`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var displays int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM panda_gallery_displays").Scan(&displays); err != nil || displays != 2 {
		t.Fatalf("displays = %d, %v", displays, err)
	}

	service := catalog.New(pool, "https://panda.test")
	list := func(unseen bool) collectorapi.CatalogResult {
		t.Helper()
		result, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, Unseen: unseen})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	displayed := map[int64]bool{}
	for _, item := range list(false).Items {
		displayed[item.GalleryID] = item.Displayed
	}
	if want := map[int64]bool{1: true, 2: true, 3: false}; !maps.Equal(displayed, want) {
		t.Fatalf("displayed = %v, want %v", displayed, want)
	}
	if unseen := list(true); len(unseen.Items) != 1 || unseen.Items[0].GalleryID != 3 {
		t.Fatalf("unseen = %+v", unseen.Items)
	}
}
