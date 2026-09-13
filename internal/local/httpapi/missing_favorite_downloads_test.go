package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/favoritedownloads"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func TestMissingFavoriteDownloadsAPI(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reads, submissions := 0, 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector-token" {
			t.Error("missing collector authentication")
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/favorites/4/download-candidates":
			reads++
			server.WriteJSON(w, 200, map[string]any{"favorites": []collectorapi.FavoriteDownloadCandidate{{Ref: panda.GalleryRef{ID: 42, Token: "token"}}}})
		case "POST /api/downloads/batch":
			submissions++
			var input struct {
				References []panda.GalleryRef `json:"references"`
			}
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			if len(input.References) != 1 || input.References[0] != (panda.GalleryRef{ID: 42, Token: "token"}) {
				t.Errorf("unexpected submission: %+v", input)
			}
			server.WriteJSON(w, 202, collectorapi.DownloadBatchCounts{NewDownloads: 1})
		default:
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
	client, err := collectorapi.NewClient(upstream.URL, "collector-token")
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), FavoriteDownloads: favoritedownloads.New(db, client)})
	base := "/api/collector/favorites/4/missing-downloads"
	var preview favoritedownloads.Preview
	if err := json.Unmarshal(request(t, h, "POST", base+"/preview", "", 200).Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.PlanID == "" || preview.NewDownloads != 1 || submissions != 0 {
		t.Fatalf("preview: %+v, submissions: %d", preview, submissions)
	}
	body := fmt.Sprintf(`{"plan_id":%q}`, preview.PlanID)
	request(t, h, "POST", base, body+`{}`, 400)
	request(t, h, "POST", "/api/collector/favorites/3/missing-downloads", body, 409)
	request(t, h, "POST", "/api/collector/favorites/all/missing-downloads/preview", "", 400)
	// A fully received confirmation remains accepted after browser cancellation.
	r := httptest.NewRequest("POST", base, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(r.Context())
	cancel()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r.WithContext(ctx))
	if w.Code != 202 {
		t.Fatalf("confirmation after disconnect: %d %s", w.Code, w.Body)
	}
	var result favoritedownloads.Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.NewDownloads != 1 || result.Category != 4 {
		t.Fatalf("result: %+v, %v", result, err)
	}
	replayed := request(t, h, "POST", base, body, 202)
	if replayed.Body.String() != w.Body.String() || submissions != 1 || reads != 1 {
		t.Fatalf("replay: %s, reads %d, submissions %d", replayed.Body, reads, submissions)
	}
}

func TestMissingFavoriteDownloadsWithoutCollector(t *testing.T) {
	h := testHandler(t)
	for _, suffix := range []string{"", "/preview"} {
		w := request(t, h, "POST", "/api/collector/favorites/0/missing-downloads"+suffix, "", 503)
		if !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unavailable: %s", w.Body)
		}
	}
}
