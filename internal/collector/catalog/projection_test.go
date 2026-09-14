package catalog

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestCompletedBackfillDoesNotReadMetadataAfterReopen(t *testing.T) {
	dir := t.TempDir()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	retain(t, db, panda.Metadata{ID: 1, Title: "Already projected"}, true)
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// Make a repeated metadata scan fail without a timing-sensitive assertion.
	if _, err := db.Exec("ALTER TABLE gallery_metadata RENAME TO unavailable_metadata"); err != nil {
		t.Fatal(err)
	}
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err != nil {
		t.Fatalf("completed backfill read metadata after reopening: %v", err)
	}
}

func TestFailedBackfillResumesAfterReopen(t *testing.T) {
	dir := t.TempDir()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	retain(t, db, panda.Metadata{ID: 1, Title: "Pending projection"}, false)
	if _, err := db.Exec("UPDATE gallery_metadata SET body = 'invalid json'"); err != nil {
		t.Fatal(err)
	}
	if err := New(db, "https://favorites.example.test").Backfill(t.Context()); err == nil {
		t.Fatal("backfill accepted invalid metadata")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, _, err = storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
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
