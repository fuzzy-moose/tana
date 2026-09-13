package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/refresh"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
)

func TestLibraryRefreshAPI(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	l, err := library.NewSQLiteRepository(db).Create(t.Context(), "Manga", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var missingID int64
	for _, name := range []string{"Gone.cbz", "Present.zip"} {
		src, err := source.NewSQLiteRepository(db).Create(t.Context(), l.ID, name, source.Archive, nil)
		if err != nil {
			t.Fatal(err)
		}
		if name == "Gone.cbz" {
			missingID = src.ID
		} else if err := os.WriteFile(filepath.Join(l.Path, name), []byte("present"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Refresh: refresh.New(db, os.DirFS)})
	var preview refresh.Preview
	for _, path := range []string{"/api/libraries/refresh", fmt.Sprintf("/api/libraries/%d/refresh", l.ID)} {
		if err := json.Unmarshal(request(t, h, "GET", path, "", 200).Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		if preview.PlanID == "" || len(preview.Candidates) != 1 || preview.Candidates[0].SourceID != missingID || preview.Candidates[0].Galleries == nil || preview.Skipped == nil {
			t.Fatalf("preview = %+v", preview)
		}
	}
	request(t, h, "GET", "/api/libraries/999999/refresh", "", 404)
	request(t, h, "GET", "/api/libraries/invalid/refresh", "", 404)
	for _, body := range []string{`{`, `{"plan_id":"x","source_ids":[1],"extra":true}`} {
		request(t, h, "POST", "/api/libraries/refresh", body, 400)
	}
	request(t, h, "POST", "/api/libraries/refresh", `{"plan_id":"unknown","source_ids":[1]}`, 409)
	body := fmt.Sprintf(`{"plan_id":%q,"source_ids":[%d]}`, preview.PlanID, missingID)
	var result refresh.Result
	if err := json.Unmarshal(request(t, h, "POST", "/api/libraries/refresh", body, 200).Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 1 || result.Removed[0] != missingID || len(result.Failed) != 0 {
		t.Fatalf("removal = %+v", result)
	}
	request(t, h, "POST", "/api/libraries/refresh", body, 409)
}
