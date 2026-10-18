package catalog

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func projectFeed(t *testing.T, db *pgxpool.Pool, entry panda.FeedEntry, at time.Time) {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token, feed_metadata_base_at) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, entry.GalleryRef.ID, entry.GalleryRef.Token, at); err != nil {
		t.Fatal(err)
	}
	if err := ProjectFeed(t.Context(), tx, entry, 1, at, at); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestProvisionalUnknownsUseFirstSightingForPagination(t *testing.T) {
	db, service := openCatalog(t)
	for id := int64(1); id <= 3; id++ {
		projectFeed(t, db, panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: id, Token: "token"}, Title: "Feed title"}, time.Unix(10+id, 123000000))
	}
	retained := panda.Metadata{ID: 4, Posted: 12, Expunged: true}
	retain(t, db, retained, true)
	first, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, first, []int64{3, 2})
	for _, item := range first.Items {
		if item.PostedAt != nil || item.PageCount != nil || item.FirstSeenAt == nil || item.MetadataState != "waiting" {
			t.Fatalf("unknown feed fields presented as known: %+v", item)
		}
	}
	next, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, next, []int64{1})
	previous, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 2, Cursor: next.PreviousCursor})
	if err != nil {
		t.Fatal(err)
	}
	assertCatalogIDs(t, previous, []int64{3, 2})
	filtered, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, Categories: []string{"manga"}})
	if err != nil || len(filtered.Items) != 0 {
		t.Fatalf("unknown category matched a category filter: %+v, %v", filtered, err)
	}
}

func TestCollectedMetadataReplacesProvisionalAndPendingRetryState(t *testing.T) {
	db, service := openCatalog(t)
	at := time.Now().UTC()
	feed := panda.FeedEntry{GalleryRef: panda.GalleryRef{ID: 1, Token: "token1"}, Title: "Provisional", Tags: []string{"artist:feed"}}
	projectFeed(t, db, feed, at)
	if _, err := db.Exec(t.Context(), `UPDATE gallery_refs SET metadata_error = 'unavailable' WHERE gallery_id = 1;
		INSERT INTO metadata_fetches (gallery_id, token, status) VALUES (1, 'wrong-token', 'pending')`); err != nil {
		t.Fatal(err)
	}
	result, err := service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10})
	if err != nil || len(result.Items) != 1 || result.Items[0].MetadataState != "failed" {
		t.Fatalf("failed entry: %+v, %v", result, err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO metadata_fetches (gallery_id, token, status) VALUES (1, 'token1', 'pending')`); err != nil {
		t.Fatal(err)
	}
	result, err = service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10})
	if err != nil || result.Items[0].MetadataState != "waiting" || result.Items[0].FetchEligibleAt != nil {
		t.Fatalf("pending explicit fetch kept failure/deferred eligibility: %+v, %v", result, err)
	}
	retain(t, db, panda.Metadata{ID: 1, Title: "Collected", Posted: 99, FileCount: 15, Category: "Manga", Tags: []string{"artist:collected"}}, true)
	feed.Title = "Later feed sighting"
	projectFeed(t, db, feed, at.Add(time.Hour))
	result, err = service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, Query: "artist:collected", Categories: []string{"manga"}})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("collected description: %+v, %v", result, err)
	}
	item := result.Items[0]
	if item.Title != "Collected" || item.MetadataSource != "collected" || item.MetadataState != "" || item.FirstSeenAt != nil || item.FetchEligibleAt != nil || item.PageCount == nil || *item.PageCount != 15 || item.PostedAt == nil || item.PostedAt.Unix() != 99 {
		t.Fatalf("incomplete replacement: %+v", item)
	}
	result, err = service.List(t.Context(), collectorapi.CatalogOptions{PageSize: 10, Query: "artist:feed"})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("feed tags survived collection: %+v, %v", result, err)
	}
}
