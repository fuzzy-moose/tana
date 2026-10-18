package storage_test

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestMigrationsCreateBaselineWithSingletons(t *testing.T) {
	pool := pgtest.Open(t, storage.Migrations())
	var versions string
	var query, categories string
	if err := pool.QueryRow(t.Context(), `SELECT
		(SELECT string_agg(version::text, ',' ORDER BY version) FROM schema_migrations),
		query, categories::text FROM panda_catalog_default_filter WHERE id = 1`).Scan(&versions, &query, &categories); err != nil {
		t.Fatal(err)
	}
	if versions != "1" || query != "" || categories != "[]" {
		t.Fatalf("versions=%q default filter=(%q, %s)", versions, query, categories)
	}
}
