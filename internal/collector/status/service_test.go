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

func TestStatusScopesFavoritesAndPartitionsInventory(t *testing.T) {
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
	service := New(db, favorites, ban, nil)
	empty, err := service.Get(t.Context())
	if err != nil || len(empty.Favorites.Categories) != 10 || empty.Inventory.GalleryReferences != 0 || len(empty.MetadataErrors) != 0 {
		t.Fatalf("empty status: %+v, %v", empty, err)
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
UPDATE metadata_retry SET last_error = 'upstream unavailable', next_attempt_at = ? WHERE id = 1;
`, time.Now().Add(time.Hour).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(2 * time.Hour)
	if err := ban.Extend(t.Context(), until); err != nil {
		t.Fatal(err)
	}
	result, err := service.Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := collectorapi.InventoryStatus{GalleryReferences: 4, MetadataAvailable: 1, MetadataPending: 2, MetadataFailed: 1, FetchesPending: 3, FetchesFailed: 1}
	if result.Inventory != want {
		t.Fatalf("inventory: %+v, want %+v", result.Inventory, want)
	}
	for i, category := range result.Favorites.Categories {
		if category.Category != i || category.State != "idle" {
			t.Fatalf("category: %+v", category)
		}
		if i == 2 {
			if category.Name != "Manga" || category.Favorites != 2 || category.LastSyncedAt == nil || category.LastSyncedAt.UnixMilli() != 1234 {
				t.Fatalf("current category: %+v", category)
			}
		} else if category.Favorites != 0 || category.LastSyncedAt != nil || category.Name != "" {
			t.Fatalf("historical data leaked: %+v", category)
		}
	}
	if result.UpstreamCooldownUntil == nil || result.UpstreamCooldownUntil.UnixMilli() != until.UnixMilli() ||
		result.MetadataLastError != "upstream unavailable" || result.MetadataRetryAt == nil ||
		len(result.MetadataErrors) != 4 || result.MetadataErrors[0].GalleryID != 4 || result.MetadataErrors[3].Error != "token_conflict" {
		t.Fatalf("diagnostics: %+v", result)
	}
}
