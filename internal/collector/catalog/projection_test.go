package catalog

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestCompletedBackfillDoesNotReadMetadataAfterReopen(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	retain(t, db, panda.Metadata{ID: 1, Title: "Already projected"}, true)
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err != nil {
		t.Fatal(err)
	}
	db = pgtest.Reopen(t, db, storage.Migrations())
	// Make a repeated metadata scan fail without a timing-sensitive assertion.
	if _, err := db.Exec(t.Context(), "ALTER TABLE gallery_metadata RENAME TO unavailable_metadata"); err != nil {
		t.Fatal(err)
	}
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err != nil {
		t.Fatalf("completed backfill read metadata after reopening: %v", err)
	}
}

func TestFailedBackfillResumesAfterReopen(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	retain(t, db, panda.Metadata{ID: 1, Title: "Pending projection"}, false)
	if _, err := db.Exec(t.Context(), "UPDATE gallery_metadata SET body = 'invalid json'::bytea"); err != nil {
		t.Fatal(err)
	}
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err == nil {
		t.Fatal("backfill accepted invalid metadata")
	}
	db = pgtest.Reopen(t, db, storage.Migrations())
	retain(t, db, panda.Metadata{ID: 1, Title: "Recovered projection"}, false)
	s := New(db, "https://favorites.example.test")
	if err := s.Backfill(t.Context()); err != nil {
		t.Fatal(err)
	}
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24})
	if err != nil || len(result.Items) != 1 || result.Items[0].Title != "Recovered projection" {
		t.Fatalf("recovered backfill: %+v, %v", result, err)
	}
}
