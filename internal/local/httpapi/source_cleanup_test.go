package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/cleanup"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type cleanupMetadata struct{}

func (cleanupMetadata) Lookup(_ context.Context, ids []int64) (collectorapi.LookupResult, error) {
	result := collectorapi.LookupResult{}
	for _, id := range ids {
		value := panda.Metadata{ID: id, Token: fmt.Sprint(id), Title: fmt.Sprintf("Version %d", id)}
		if id == 2 {
			value.ParentID, value.ParentToken = 1, "1"
		}
		result.Galleries = append(result.Galleries, collectorapi.CollectedMetadata{Metadata: value})
	}
	return result, nil
}

func TestSourceCleanupAPI(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	roots := []string{t.TempDir(), t.TempDir()}
	var sources []source.Source
	for i, root := range roots {
		lib, err := library.NewSQLiteRepository(db).Create(t.Context(), fmt.Sprintf("Library %d", i), root)
		if err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("Version [%d].cbz", i+1)
		// Presence alone suffices; cleanup must not inspect archive contents.
		if err := os.WriteFile(filepath.Join(root, name), []byte("archive placeholder"), 0o600); err != nil {
			t.Fatal(err)
		}
		src, err := source.NewSQLiteRepository(db).Create(t.Context(), lib.ID, name, source.Archive, []string{"1.jpg"})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, src)
	}
	h := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Cleanup: cleanup.New(db, cleanupMetadata{})})
	var preview cleanup.Preview
	if err := json.Unmarshal(request(t, h, "GET", "/api/source-cleanup", "", 200).Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.PlanID == "" || len(preview.Candidates) != 1 || preview.Candidates[0].Source.ID != sources[0].ID || preview.Candidates[0].Replacement.ID != sources[1].ID {
		t.Fatalf("unexpected global preview: %+v", preview)
	}
	oldPath := filepath.Join(roots[0], sources[0].Path)
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("preview deleted archive: %v", err)
	}
	for _, body := range []string{`{`, `{"plan_id":"x","source_ids":[1],"extra":true}`} {
		request(t, h, "POST", "/api/source-cleanup", body, 400)
	}
	// A cross-origin attempt cannot consume the confirmation or delete files.
	body := fmt.Sprintf(`{"plan_id":%q,"source_ids":[%d]}`, preview.PlanID, sources[0].ID)
	r := httptest.NewRequest("POST", "/api/source-cleanup", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://other.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-origin deletion: %d %s", w.Code, w.Body)
	}
	var result cleanup.Result
	if err := json.Unmarshal(request(t, h, "POST", "/api/source-cleanup", body, 200).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0] != sources[0].ID || len(result.Failed) != 0 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("old archive remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(roots[1], sources[1].Path)); err != nil {
		t.Fatalf("replacement missing: %v", err)
	}
	request(t, h, "POST", "/api/source-cleanup", body, 409)
}

func TestSourceCleanupWithoutCollector(t *testing.T) {
	h := testHandler(t)
	for _, method := range []string{"GET", "POST"} {
		body := ""
		if method == "POST" {
			body = `{"plan_id":"unknown","source_ids":[1]}`
		}
		w := request(t, h, method, "/api/source-cleanup", body, 503)
		if !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unexpected unavailable response: %s", w.Body)
		}
	}
}
