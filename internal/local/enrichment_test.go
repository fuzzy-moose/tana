package local_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestAppImportsBeforeCollectorRespondsThenEnriches(t *testing.T) {
	release := make(chan struct{})
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			IDs []int64 `json:"gallery_ids"`
		}
		err := json.NewDecoder(r.Body).Decode(&input)
		if err != nil || r.URL.Path != "/api/metadata/lookup" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-token" || !reflect.DeepEqual(input.IDs, []int64{396226}) {
			t.Errorf("unexpected collector request: %s %s, IDs=%v error=%v", r.Method, r.URL.Path, input.IDs, err)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-release:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{{Metadata: panda.Metadata{ID: 396226, Title: "Panda title"}}}})
	}))
	defer collector.Close()
	t.Setenv("TANA_DATA_DIR", t.TempDir())
	t.Setenv("TANA_COLLECTOR_URL", collector.URL)
	t.Setenv("TANA_COLLECTOR_API_TOKEN", "test-token")
	root := filepath.Join(t.TempDir(), "Source [396226]")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"1.jpg": "image", "galleryinfo.txt": "Title: Snapshot\nfooter"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app, err := local.New(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	l, err := app.Libraries.Create(t.Context(), "Comics", root)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Scans.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for app.Scans.Status().Phase != "completed" {
		if time.Now().After(deadline) {
			t.Fatalf("import blocked: %+v", app.Scans.Status())
		}
		time.Sleep(time.Millisecond)
	}
	galleries, err := app.Galleries.List(t.Context())
	if err != nil || len(galleries) != 1 || galleries[0].Title != "Snapshot" {
		t.Fatalf("initial metadata: %+v, %v", galleries, err)
	}
	close(release)
	deadline = time.Now().Add(5 * time.Second)
	for {
		g, err := app.Galleries.Get(t.Context(), galleries[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if g.Title == "Panda title" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("gallery not enriched: %+v", g)
		}
		time.Sleep(time.Millisecond)
	}
}
