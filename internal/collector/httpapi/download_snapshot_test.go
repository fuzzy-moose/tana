package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestCompletedDownloadsSnapshotAPI(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	logger := slog.New(slog.DiscardHandler)
	service, err := downloads.New(t.Context(), db, t.TempDir(), waitingArchiveClient{}, nil, logger, downloads.StorageConfig{PauseBelowBytes: 1, ResumeAtBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	service.Close()
	handler := NewHandler(&collector.App{Logger: logger, Downloads: service, APIToken: "test-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/downloads/completed", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated snapshot: %d %s", w.Code, w.Body)
	}
	w = apiRequest(handler, http.MethodGet, "/api/downloads/completed", "")
	var result struct {
		GalleryIDs []int64 `json:"gallery_ids"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.GalleryIDs == nil || len(result.GalleryIDs) != 0 {
		t.Fatalf("empty snapshot: %d %s, %v", w.Code, w.Body, err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at)
		VALUES (1, 'secret', 'completed', to_timestamp(1), to_timestamp(1)), (2, 'secret', 'queued', to_timestamp(2), to_timestamp(2)),
		(3, 'secret', 'completed', to_timestamp(3), to_timestamp(3))`); err != nil {
		t.Fatal(err)
	}
	w = apiRequest(handler, http.MethodGet, "/api/downloads/completed", "")
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || !slices.Equal(result.GalleryIDs, []int64{3, 1}) {
		t.Fatalf("completed snapshot: %d %s, %v", w.Code, w.Body, err)
	}
}
