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
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestStatusViewsScopeFavoritesAndPartitionInventory(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	favorites := favorites.New(ctx, db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "current"}, nil, slog.New(slog.DiscardHandler))
	defer favorites.Close()
	ban := pandaban.New(db)
	service := New(db, favorites, ban, pandaban.NewAuthenticated(db), nil, nil, nil)
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
	_, err = db.Exec(t.Context(), `
INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at, metadata_error) VALUES
  (1, 'one', to_timestamp(1), 'refresh failed'), (2, 'two', NULL, NULL),
  (3, 'three', to_timestamp(2), 'gallery unavailable'), (4, 'four', to_timestamp(3), 'retry requested');
INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{}', to_timestamp(0.5));
INSERT INTO metadata_fetches (gallery_id, token, status, error) VALUES
  (1, 'one', 'pending', ''), (4, 'four', 'pending', ''), (99, 'unknown', 'pending', ''),
  (100, 'unknown', 'failed', 'token_conflict');
INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES
  (1, 'https://panda.test', 'current', 2, 'Manga', to_timestamp(1.234)),
  (2, 'https://panda.test', 'historical', 2, 'Old', to_timestamp(1)),
  (3, 'https://other.test', 'current', 0, 'Other host', to_timestamp(1));
INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES
  (1, 1, 'one', to_timestamp(1)), (1, 2, 'two', to_timestamp(2)), (2, 3, 'three', to_timestamp(3)), (3, 4, 'four', to_timestamp(4));
INSERT INTO favorite_syncs (category_id, state, "full", queued_at) VALUES (1, 'running', false, to_timestamp(1));
`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(t.Context(), `UPDATE metadata_retry SET last_error = 'upstream unavailable', next_attempt_at = $1 WHERE id = 1`, time.Now().Add(time.Hour))
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
	if favoriteStatus.AuthenticatedCooldownUntil != nil {
		t.Fatalf("main cooldown leaked to authenticated requests: %+v", favoriteStatus)
	}
	for i, category := range favoriteStatus.Categories {
		if category.Category != i {
			t.Fatalf("category: %+v", category)
		}
		if i == 2 {
			if category.Name != "Manga" || category.Favorites != 2 || category.LastSyncedAt == nil || category.LastSyncedAt.UnixMilli() != 1234 {
				t.Fatalf("current category: %+v", category)
			}
			if category.State != "running" || category.RetryAt != nil {
				t.Fatalf("main cooldown paused favorites: %+v", category)
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

func TestAuthenticatedCooldownUpdatesFavoritesOnly(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	favorites := favorites.New(ctx, db, panda.AuthenticatedConfig{FavoritesURL: "https://panda.test", AccountKey: "current"}, nil, slog.New(slog.DiscardHandler))
	defer favorites.Close()
	_, err := db.Exec(t.Context(), `INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES (1, 'https://panda.test', 'current', 2, 'Manga', to_timestamp(1));
		INSERT INTO favorite_syncs (category_id, state, "full", queued_at) VALUES (1, 'running', false, to_timestamp(1));`)
	if err != nil {
		t.Fatal(err)
	}
	ban := pandaban.NewAuthenticated(db)
	until := time.Now().Add(time.Hour)
	if err := ban.Extend(t.Context(), until); err != nil {
		t.Fatal(err)
	}
	service := New(db, favorites, pandaban.New(db), ban, nil, nil, nil)
	result, err := service.Favorites(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	category := result.Categories[2]
	if result.AuthenticatedCooldownUntil == nil || result.AuthenticatedCooldownUntil.UnixMilli() != until.UnixMilli() ||
		category.State != "waiting_cooldown" || category.RetryAt == nil || category.RetryAt.UnixMilli() != until.UnixMilli() {
		t.Fatalf("authenticated cooldown missing from favorites: %+v, %+v", result, category)
	}
	metadata, err := service.Metadata(t.Context())
	if err != nil || metadata.UpstreamCooldownUntil != nil {
		t.Fatalf("authenticated cooldown leaked to metadata: %+v, %v", metadata, err)
	}
}

func TestInventoryCountsRetainedMetadataAndMatchingRetries(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	_, err := db.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at) VALUES
		(1, 'one', NULL), (2, 'two', NULL), (3, 'three', to_timestamp(1)), (4, 'four', to_timestamp(1)), (5, 'five', to_timestamp(1));
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{}', to_timestamp(1)), (5, '{}', to_timestamp(1));
		INSERT INTO metadata_fetches (gallery_id, token, status) VALUES
		(2, 'two', 'pending'), (2, 'wrong', 'pending'), (3, 'three', 'pending'),
		(4, 'wrong', 'pending'), (5, 'five', 'pending');`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(db, nil, nil, nil, nil, nil, nil).Inventory(t.Context())
	want := collectorapi.InventoryStatus{GalleryReferences: 5, MetadataAvailable: 2, MetadataPending: 2, MetadataFailed: 1, FetchesPending: 5}
	if err != nil || got != want {
		t.Fatalf("inventory: %+v, %v; want %+v", got, err, want)
	}
}

func TestRecentMetadataErrorsMergeLatestDistinctGalleries(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	_, err := db.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token, metadata_attempted_at, metadata_error)
		SELECT id, 'token', to_timestamp(100 + id), 'gallery error' FROM generate_series(1, 30) AS id;
		INSERT INTO metadata_fetch_jobs (id, created_at, completed_at) VALUES
		('new', to_timestamp(1000), to_timestamp(1001)), ('old', to_timestamp(1), to_timestamp(2));
		INSERT INTO metadata_fetches (id, gallery_id, token, status, error) VALUES
		(1, 10, 'token', 'failed', 'new fetch error'), (2, 31, 'token', 'failed', 'new fetch error'),
		(3, 31, 'other', 'failed', 'old fetch error'), (4, 30, 'token', 'failed', 'old fetch error');
		INSERT INTO metadata_fetch_job_entries (job_id, position, fetch_id) VALUES
		('new', 1, 1), ('new', 2, 2), ('old', 1, 3), ('old', 2, 4);`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := New(db, nil, pandaban.New(db), nil, nil, nil, nil).Metadata(t.Context())
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
