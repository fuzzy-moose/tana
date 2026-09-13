package catalog

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLookupReturnsRetainedMetadataAndInventoryReferences(t *testing.T) {
	db, s := openCatalog(t)
	entry := panda.Metadata{ID: 1, Token: "token1", Title: "Retained", Expunged: true,
		TitleJapanese: "日本語", Uploader: "Uploader", Tags: []string{"artist:someone"},
		ParentID: 3, ParentToken: "parent", Torrents: []panda.Torrent{{Hash: "hash", Name: "torrent"}}}
	retain(t, db, entry, true)
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at, metadata_error)
		VALUES (2, 'pending/token', 20, 'unavailable');
		UPDATE gallery_metadata SET refreshed_at = 1234 WHERE gallery_id = 1`); err != nil {
		t.Fatal(err)
	}
	result, err := s.Lookup(t.Context(), 1)
	if err != nil || result.Metadata == nil || !reflect.DeepEqual(*result.Metadata, entry) ||
		result.RefreshedAt == nil || result.RefreshedAt.UnixMilli() != 1234 || result.Token != "token1" || result.Unverified || result.FetchJob != nil {
		t.Fatalf("retained metadata = %+v, %v", result, err)
	}
	result, err = s.Lookup(t.Context(), 2)
	if err != nil || result.GalleryID != 2 || result.Token != "pending/token" ||
		result.URL != "https://favorites.example.test/g/2/pending%2Ftoken/" || result.Metadata != nil || result.RefreshedAt != nil || result.Unverified || result.FetchJob != nil {
		t.Fatalf("known reference = %+v, %v", result, err)
	}
	page, err := s.List(t.Context(), collectorapi.CatalogOptions{Page: 1, PageSize: 24, IncludeExpunged: true})
	retained, lookupErr := s.Lookup(t.Context(), 1)
	if err != nil || lookupErr != nil || len(page.Items) != 1 || page.Items[0].URL != retained.URL {
		t.Fatalf("link convention = %+v, %+v, %v, %v", page, retained, err, lookupErr)
	}
	if _, err := s.Lookup(t.Context(), 99); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing reference = %v", err)
	}
	if _, err := s.Lookup(t.Context(), 0); !errors.Is(err, ErrInvalidGalleryReference) {
		t.Fatalf("invalid reference = %v", err)
	}
}
