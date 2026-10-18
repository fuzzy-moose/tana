package storage_test

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestMigrationsCreateBaselineWithSingletons(t *testing.T) {
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
	if len(versions) != 1 || versions[0] != 1 {
		t.Fatalf("schema versions = %v, want [1]", versions)
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
