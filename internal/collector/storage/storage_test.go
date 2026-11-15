package storage_test

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
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

func TestDisplayEvidenceMarksDisplaysFromBeforeVerdicts(t *testing.T) {
	all := storage.Migrations()
	upToFive := fstest.MapFS{}
	for _, name := range []string{"0001_baseline.sql", "0002_panda_gallery_displays.sql", "0003_panda_catalog_catch_up.sql",
		"0004_panda_gallery_verdicts.sql", "0005_panda_score_overrides.sql"} {
		data, err := fs.ReadFile(all, name)
		if err != nil {
			t.Fatal(err)
		}
		upToFive[name] = &fstest.MapFile{Data: data}
	}
	pool := pgtest.Open(t, upToFive)
	if _, err := pool.Exec(t.Context(), `INSERT INTO panda_gallery_displays (gallery_id, displayed_at)
		SELECT 1, applied_at - interval '1 hour' FROM schema_migrations WHERE version = 4
		UNION ALL SELECT 2, applied_at + interval '1 hour' FROM schema_migrations WHERE version = 4`); err != nil {
		t.Fatal(err)
	}
	pool = pgtest.Reopen(t, pool, all)
	var older, newer bool
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT evidence FROM panda_gallery_displays WHERE gallery_id = 1),
		(SELECT evidence FROM panda_gallery_displays WHERE gallery_id = 2)`).Scan(&older, &newer); err != nil {
		t.Fatal(err)
	}
	if older || !newer {
		t.Fatalf("evidence: older=%v newer=%v", older, newer)
	}
}
