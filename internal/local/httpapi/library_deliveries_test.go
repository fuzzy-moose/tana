package httpapi

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/delivery"
)

func TestLibraryDeliveryAPITransfersImportsThenDeletes(t *testing.T) {
	var archive bytes.Buffer
	z := zip.NewWriter(&archive)
	page, err := z.Create("1.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.Write([]byte("inventory only")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls []string
	var app *local.App
	root := t.TempDir()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("Cookie") != "" {
			t.Error("incorrect collector credentials")
		}
		switch r.URL.Path {
		case "/api/downloads/completed":
			fmt.Fprint(w, `{"gallery_ids":[7,8]}`)
		case "/api/downloads/9":
			fmt.Fprint(w, `{"gallery_id":9,"state":"queued"}`)
		case "/api/downloads/7/file", "/api/downloads/8/file":
			mu.Lock()
			calls = append(calls, r.URL.Path)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/zip")
			w.Write(archive.Bytes())
		case "/api/downloads/7", "/api/downloads/8":
			if r.Method != http.MethodDelete {
				t.Error("expected collector deletion")
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			id := strings.TrimPrefix(r.URL.Path, "/api/downloads/")
			if content, err := os.ReadFile(filepath.Join(root, "["+id+"].zip")); err != nil || !bytes.Equal(content, archive.Bytes()) {
				t.Errorf("collector deletion before safe save: %v", err)
			}
			galleries, err := app.Galleries.List(r.Context())
			want := 1
			if id == "8" {
				want = 2
			}
			if err != nil || len(galleries) != want {
				t.Errorf("collector deletion before import: %+v, %v", galleries, err)
			}
			calls = append(calls, "delete "+id)
			w.WriteHeader(http.StatusNoContent)
		default:
			// Metadata enrichment is independent of delivery completion.
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer upstream.Close()
	app, err = local.New(t.Context(), local.Config{DataDir: t.TempDir(), CollectorURL: upstream.URL, CollectorAPIToken: "test-token"}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	lib, err := app.Libraries.Create(t.Context(), "Comics", root)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(app)
	request(t, h, "POST", "/api/library-deliveries", fmt.Sprintf(`{"library_id":%d,"gallery_id":9}`, lib.ID), 409)
	for _, body := range []string{`{`, `{}`, `{"library_id":1,"all":true,"gallery_id":7}`, `{"library_id":1,"all":true,"extra":true}`} {
		request(t, h, "POST", "/api/library-deliveries", body, 400)
	}
	body := fmt.Sprintf(`{"library_id":%d,"all":true}`, lib.ID)
	crossOrigin := httptest.NewRequest(http.MethodPost, "/api/library-deliveries", strings.NewReader(body))
	crossOrigin.Header.Set("Content-Type", "application/json")
	crossOrigin.Header.Set("Origin", "https://other.example")
	blocked := httptest.NewRecorder()
	h.ServeHTTP(blocked, crossOrigin)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("cross-origin delivery accepted: %d", blocked.Code)
	}
	w := request(t, h, "POST", "/api/library-deliveries", body, 202)
	var batch delivery.Batch
	if err := json.Unmarshal(w.Body.Bytes(), &batch); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Location") != fmt.Sprintf("/api/library-deliveries/%d", batch.ID) || len(batch.Items) != 2 {
		t.Fatalf("incorrect snapshot: %s", w.Body)
	}
	deadline := time.Now().Add(5 * time.Second)
	var listed struct {
		Batches []delivery.Batch `json:"batches"`
	}
	for time.Now().Before(deadline) {
		w = request(t, h, "GET", "/api/library-deliveries", "", 200)
		if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		if len(listed.Batches) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(listed.Batches) != 0 {
		t.Fatalf("delivery did not complete: %+v", listed.Batches)
	}
	request(t, h, "GET", fmt.Sprintf("/api/library-deliveries/%d", batch.ID), "", 404)
	mu.Lock()
	defer mu.Unlock()
	want := []string{"/api/downloads/7/file", "delete 7", "/api/downloads/8/file", "delete 8"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("nonsequential delivery: %v", calls)
	}
	request(t, h, "GET", "/api/library-deliveries", "", 200)
	request(t, h, "GET", "/api/library-deliveries/999999", "", 404)
}

func TestLibraryDeliveryWithoutCollector(t *testing.T) {
	h := testHandler(t)
	for _, path := range []string{"/api/library-deliveries", "/api/library-deliveries/1"} {
		request(t, h, "GET", path, "", 503)
	}
	request(t, h, "POST", "/api/library-deliveries", `{"library_id":1,"all":true}`, 503)
}
