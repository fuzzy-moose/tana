package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestGalleryLookupSchedulesOnlyPastedTokenFallback(t *testing.T) {
	handler, db := pausedHandler(t)
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'known');
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES
		(1, '{"gid":1,"token":"known","title":"Retained","expunged":true}', 1000);
		INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'reference-only');`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body     string
		id       int64
		token    string
		metadata bool
	}{
		{`{"gid":1,"token":"ignored"}`, 1, "known", true},
		{`{"gid":2}`, 2, "reference-only", false},
		{`{"gid":2,"token":"ignored"}`, 2, "reference-only", false},
	} {
		w := apiRequest(handler, http.MethodPost, "/api/catalog/lookup", tc.body)
		var result collectorapi.GalleryLookupResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK ||
			result.GalleryID != tc.id || result.Token != tc.token || (result.Metadata != nil) != tc.metadata || result.Unverified || result.FetchJob != nil {
			t.Fatalf("known lookup: %d %s, %v", w.Code, w.Body, err)
		}
	}
	w := apiRequest(handler, http.MethodPost, "/api/catalog/lookup", `{"gid":3}`)
	if w.Code != http.StatusNotFound || w.Body.String() != "{\"error\":\"gallery_not_found\"}\n" {
		t.Fatalf("missing lookup: %d %s", w.Code, w.Body)
	}
	var jobs int
	if err := db.QueryRow(`SELECT count(*) FROM metadata_fetch_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("stored lookup scheduled jobs: %d, %v", jobs, err)
	}
	w = apiRequest(handler, http.MethodPost, "/api/catalog/lookup", `{"gid":3,"token":"pasted/token"}`)
	var result collectorapi.GalleryLookupResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusAccepted ||
		result.GalleryID != 3 || result.Token != "pasted/token" || result.URL != "https://favorites.example.test/g/3/pasted%2Ftoken/" ||
		!result.Unverified || result.Metadata != nil || result.FetchJob == nil || result.FetchJob.Status != "pending" ||
		len(result.FetchJob.Entries) != 1 || result.FetchJob.Entries[0].GalleryID != 3 {
		t.Fatalf("fallback lookup: %d %s, %v", w.Code, w.Body, err)
	}
	var references int
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM gallery_refs), (SELECT count(*) FROM metadata_fetch_jobs)`).Scan(&references, &jobs); err != nil || references != 2 || jobs != 1 {
		t.Fatalf("fallback admission: refs=%d jobs=%d, %v", references, jobs, err)
	}
	w = apiRequest(handler, http.MethodGet, "/api/metadata/fetches/"+result.FetchJob.ID, "")
	if w.Code != http.StatusOK {
		t.Fatalf("durable job poll: %d %s", w.Code, w.Body)
	}
}

func TestGalleryLookupRejectsInvalidReferencesAndStorageFailures(t *testing.T) {
	handler, db := pausedHandler(t)
	for _, body := range []string{`{}`, `{"gid":0}`, `{"gid":-1}`, `{"gid":1,"token":"two words"}`} {
		w := apiRequest(handler, http.MethodPost, "/api/catalog/lookup", body)
		if w.Code != http.StatusBadRequest || w.Body.String() != "{\"error\":\"invalid_gallery_reference\"}\n" {
			t.Fatalf("invalid reference: %d %s", w.Code, w.Body)
		}
	}
	// A failed stored read must not become a submission, even with a pasted token.
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'known');
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, 'invalid JSON', 1000)`); err != nil {
		t.Fatal(err)
	}
	w := apiRequest(handler, http.MethodPost, "/api/catalog/lookup", `{"gid":1,"token":"pasted"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("corrupt metadata became a lookup result: %d %s", w.Code, w.Body)
	}
	var jobs int
	if err := db.QueryRow(`SELECT count(*) FROM metadata_fetch_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("failed lookup scheduled jobs: %d, %v", jobs, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	w = apiRequest(handler, http.MethodPost, "/api/catalog/lookup", `{"gid":3,"token":"pasted"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("database failure became not found: %d %s", w.Code, w.Body)
	}
}
