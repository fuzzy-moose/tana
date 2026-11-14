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
