package status

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestStatusViewsScopeFavoritesAndPartitionInventory(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	favorites := favorites.New(ctx, db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "current"}, nil, slog.New(slog.DiscardHandler))
	defer favorites.Close()
	ban := pandaban.New(db)
	service := New(db, favorites, ban)
	emptyFavorites, err := service.Favorites(t.Context())
	if err != nil || len(emptyFavorites.Categories) != 10 {
		t.Fatalf("empty favorites: %+v, %v", emptyFavorites, err)
	}
	emptyInventory, err := service.Inventory(t.Context())
	if err != nil || emptyInventory.GalleryReferences != 0 {
		t.Fatalf("empty inventory: %+v, %v", emptyInventory, err)
	}
	emptyMetadata, err := service.Metadata(t.Context())
	if err != nil || len(emptyMetadata.MetadataErrors) != 0 {
		t.Fatalf("empty metadata: %+v, %v", emptyMetadata, err)
	}
	_, err = db.Exec(`
INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at, metadata_error) VALUES
  (1, 'one', 1000, 'refresh failed'), (2, 'two', NULL, NULL),
  (3, 'three', 2000, 'gallery unavailable'), (4, 'four', 3000, 'retry requested');
INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{}', 500);
INSERT INTO metadata_fetches (gallery_id, token, status, error) VALUES
  (1, 'one', 'pending', ''), (4, 'four', 'pending', ''), (99, 'unknown', 'pending', ''),
  (100, 'unknown', 'failed', 'token_conflict');
INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES
  (1, 'https://panda.test', 'current', 2, 'Manga', 1234),
  (2, 'https://panda.test', 'historical', 2, 'Old', 1000),
  (3, 'https://other.test', 'current', 0, 'Other host', 1000);
INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES
  (1, 1, 'one', 1), (1, 2, 'two', 2), (2, 3, 'three', 3), (3, 4, 'four', 4);
INSERT INTO favorite_syncs (category_id, state, full, queued_at) VALUES (1, 'running', 0, 1000);
UPDATE metadata_retry SET last_error = 'upstream unavailable', next_attempt_at = ? WHERE id = 1;
`, time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(2 * time.Hour)
	if err := ban.Extend(t.Context(), until); err != nil {
		t.Fatal(err)
	}
	inventory, err := service.Inventory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := collectorapi.InventoryStatus{GalleryReferences: 4, MetadataAvailable: 1, MetadataPending: 2, MetadataFailed: 1, FetchesPending: 3, FetchesFailed: 1}
	if inventory != want {
		t.Fatalf("inventory: %+v, want %+v", inventory, want)
	}
	favoriteStatus, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i, category := range favoriteStatus.Categories {
		if category.Category != i {
			t.Fatalf("category: %+v", category)
		}
		if i == 2 {
			if category.Name != "Manga" || category.Favorites != 2 || category.LastSyncedAt == nil || category.LastSyncedAt.UnixMilli() != 1234 {
				t.Fatalf("current category: %+v", category)
			}
			if category.State != "waiting_cooldown" || category.RetryAt == nil || category.RetryAt.UnixMilli() != until.UnixMilli() {
				t.Fatalf("shared cooldown missing from favorites: %+v", category)
			}
		} else if category.State != "idle" || category.Favorites != 0 || category.LastSyncedAt != nil || category.Name != "" {
			t.Fatalf("historical data leaked: %+v", category)
		}
	}
	result, err := service.Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.UpstreamCooldownUntil == nil || result.UpstreamCooldownUntil.UnixMilli() != until.UnixMilli() ||
		result.MetadataLastError != "upstream unavailable" || result.MetadataRetryAt == nil ||
		len(result.MetadataErrors) != 4 || result.MetadataErrors[0].GalleryID != 4 || result.MetadataErrors[3].Error != "token_conflict" {
		t.Fatalf("diagnostics: %+v", result)
	}
}

func TestInventoryCountsRetainedMetadataAndMatchingRetries(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at) VALUES
		(1, 'one', NULL), (2, 'two', NULL), (3, 'three', 1), (4, 'four', 1), (5, 'five', 1);
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{}', 1), (5, '{}', 1);
		INSERT INTO metadata_fetches (gallery_id, token, status) VALUES
		(2, 'two', 'pending'), (2, 'wrong', 'pending'), (3, 'three', 'pending'),
		(4, 'wrong', 'pending'), (5, 'five', 'pending');`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(db, nil, nil).Inventory(t.Context())
	want := collectorapi.InventoryStatus{GalleryReferences: 5, MetadataAvailable: 2, MetadataPending: 2, MetadataFailed: 1, FetchesPending: 5}
	if err != nil || got != want {
		t.Fatalf("inventory: %+v, %v; want %+v", got, err, want)
	}
}

func TestRecentMetadataErrorsMergeLatestDistinctGalleries(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`WITH RECURSIVE ids(id) AS (VALUES(1) UNION ALL SELECT id + 1 FROM ids WHERE id < 30)
		INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at, metadata_error)
		SELECT id, 'token', 100 + id, 'gallery error' FROM ids;
		INSERT INTO metadata_fetch_jobs (id, created_at, completed_at) VALUES ('new', 1000, 1001), ('old', 1, 2);
		INSERT INTO metadata_fetches (id, gallery_id, token, status, error) VALUES
		(1, 10, 'token', 'failed', 'new fetch error'), (2, 31, 'token', 'failed', 'new fetch error'),
		(3, 31, 'other', 'failed', 'old fetch error'), (4, 30, 'token', 'failed', 'old fetch error');
		INSERT INTO metadata_fetch_job_entries (job_id, position, fetch_id) VALUES
		('new', 1, 1), ('new', 2, 2), ('old', 1, 3), ('old', 2, 4);`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(db, nil, pandaban.New(db)).Metadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{10, 31, 30, 29, 28, 27, 26, 25, 24, 23}
	if len(got.MetadataErrors) != len(want) {
		t.Fatalf("recent errors: %+v", got.MetadataErrors)
	}
	for i, id := range want {
		if got.MetadataErrors[i].GalleryID != id {
			t.Fatalf("recent errors: %+v; want gallery IDs %v", got.MetadataErrors, want)
		}
	}
	if got.MetadataErrors[0].Error != "new fetch error" || got.MetadataErrors[1].Error != "new fetch error" || got.MetadataErrors[2].Error != "gallery error" {
		t.Fatalf("older error replaced latest outcome: %+v", got.MetadataErrors)
	}
}
