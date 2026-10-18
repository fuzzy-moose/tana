package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestReferenceImportPauseResumeAPI(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	logger := slog.New(slog.DiscardHandler)
	service, err := metadata.NewReferenceImports(t.Context(), db, t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	service.Close() // Exercise HTTP state transitions without worker scheduling.
	if _, err := db.Exec(t.Context(), `INSERT INTO reference_imports
		(id, filename, status, created_at, size_bytes, processed_bytes, pending)
		VALUES ('accepted', 'references.txt', 'validating', to_timestamp(1), 42, 42, 1)`); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&collector.App{Logger: logger, ReferenceImports: service, APIToken: "test-token"})
	const path = "/api/reference-imports/accepted"
	for _, action := range []string{"pause", "pause", "resume", "resume"} {
		w := apiRequest(handler, http.MethodPost, path+"/"+action, "")
		var result collectorapi.ReferenceImport
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK ||
			result.ID != "accepted" || result.Status != "validating" || result.Paused != (action == "pause") ||
			result.ProcessedBytes != 42 || result.Pending != 1 {
			t.Fatalf("%s: %d %s, %v", action, w.Code, w.Body, err)
		}
		w = apiRequest(handler, http.MethodGet, path, "")
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK || result.Paused != (action == "pause") {
			t.Fatalf("get after %s: %d %s, %v", action, w.Code, w.Body, err)
		}
	}
	for _, action := range []string{"pause", "resume"} {
		w := apiRequest(handler, http.MethodPost, "/api/reference-imports/missing/"+action, "")
		if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "import_not_found") {
			t.Fatalf("%s missing: %d %s", action, w.Code, w.Body)
		}
	}
	w := apiRequest(handler, http.MethodPost, path+"/cancel", "")
	if w.Code != http.StatusOK {
		t.Fatalf("cancel: %d %s", w.Code, w.Body)
	}
	for _, action := range []string{"pause", "resume"} {
		w := apiRequest(handler, http.MethodPost, path+"/"+action, "")
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "import_state_conflict") {
			t.Fatalf("%s cancelled: %d %s", action, w.Code, w.Body)
		}
	}
}

func TestReferenceImportListTotalIgnoresPagination(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	logger := slog.New(slog.DiscardHandler)
	service, err := metadata.NewReferenceImports(t.Context(), db, t.TempDir(), logger)
	if err != nil {
		t.Fatal(err)
	}
	service.Close()
	if _, err := db.Exec(t.Context(), `INSERT INTO reference_imports (id, filename, status, created_at, size_bytes) VALUES
		('a', 'a.txt', 'processing', to_timestamp(1), 1), ('b', 'b.txt', 'processing', to_timestamp(2), 1), ('c', 'c.txt', 'processing', to_timestamp(3), 1)`); err != nil {
		t.Fatal(err)
	}
	// Completed beyond the retention window: excluded from both the list and the total.
	if _, err := db.Exec(t.Context(), `INSERT INTO reference_imports (id, filename, status, created_at, completed_at, size_bytes)
		VALUES ('old', 'old.txt', 'completed', to_timestamp(0.5), to_timestamp(0.6), 1)`); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&collector.App{Logger: logger, ReferenceImports: service, APIToken: "test-token"})
	for _, tc := range []struct {
		query string
		count int
	}{{"?limit=2", 2}, {"?limit=2&offset=2", 1}, {"?limit=2&offset=10", 0}} {
		w := apiRequest(handler, http.MethodGet, "/api/reference-imports"+tc.query, "")
		var result collectorapi.ReferenceImportList
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK ||
			result.Imports == nil || len(result.Imports) != tc.count || result.Total != 3 {
			t.Fatalf("list %s: %d %s, %v", tc.query, w.Code, w.Body, err)
		}
	}
}
