package collector

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestAppLifecycle(t *testing.T) {
	for _, override := range []bool{false, true} {
		name := "default_download_directory"
		if override {
			name = "explicit_download_directory"
		}
		t.Run(name, func(t *testing.T) {
			body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><link href="https://example.test/g/42/token/"/></entry></feed>`)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_, _ = io.WriteString(w, `{"gmetadata":[{"gid":42,"token":"token","title":"Collected title"}]}`)
					return
				}
				_, _ = w.Write(body)
			}))
			defer upstream.Close()
			dir := t.TempDir()
			t.Setenv("TANA_COLLECTOR_DATA_DIR", dir)
			downloadDir := filepath.Join(dir, "downloads")
			configuredDownloadDir := ""
			if override {
				downloadDir = filepath.Join(t.TempDir(), "archives")
				configuredDownloadDir = downloadDir
			}
			t.Setenv("TANA_COLLECTOR_DOWNLOAD_DIR", configuredDownloadDir)
			t.Setenv("TANA_COLLECTOR_API_TOKEN", "test-token")
			t.Setenv("PANDA_FEED_URL", upstream.URL)
			t.Setenv("PANDA_FEED_INTERVAL", "1h")
			t.Setenv("PANDA_FEED_RETRY_DELAY", "1m")
			t.Setenv("PANDA_API_URL", upstream.URL)
			t.Setenv("PANDA_RATE_INTERVAL", "2500ms")
			t.Setenv("PANDA_FAVORITES_URL", upstream.URL+"/account/saved-items")
			t.Setenv("PANDA_ARCHIVER_URL", upstream.URL+"/account/prepare-archive")
			t.Setenv("PANDA_FAVORITES_COOKIES", `{"ipb_member_id":"1","ipb_pass_hash":"test-hash","sp":"1"}`)
			t.Setenv("PANDA_FAVORITES_ACCOUNT_KEY", "")
			cfg, err := LoadConfig(os.Getenv)
			if err != nil {
				t.Fatal(err)
			}
			app, err := New(t.Context(), cfg, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			if info, err := os.Stat(downloadDir); err != nil || !info.IsDir() {
				t.Fatalf("download directory not created at %q: %v", downloadDir, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				var n int
				if err := app.db.QueryRow("SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL").Scan(&n); err != nil {
					t.Fatal(err)
				}
				collected, err := app.Metadata.Get(t.Context(), 42)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					t.Fatal(err)
				}
				if n == 1 && err == nil && collected.Metadata.Title == "Collected title" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("collector did not capture feed and collect metadata")
				}
				time.Sleep(time.Millisecond)
			}
			catalog, err := app.Catalog.List(t.Context(), collectorapi.CatalogOptions{Query: "title:collected", Page: 1, PageSize: 24})
			if err != nil || catalog.Total != 1 || len(catalog.Items) != 1 || catalog.Items[0].URL != upstream.URL+"/g/42/token/" {
				t.Fatalf("collected feed metadata absent from catalog: %+v, %v", catalog, err)
			}
			if err := app.Close(); err != nil {
				t.Fatal(err)
			}
			db, _, err := storage.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var saved []byte
			if err := db.QueryRow("SELECT body FROM raw_feeds").Scan(&saved); err != nil || !bytes.Equal(saved, body) {
				t.Fatalf("raw feed did not survive close: %q, %v", saved, err)
			}
			var id int64
			var token string
			if err := db.QueryRow("SELECT gallery_id, token FROM gallery_refs").Scan(&id, &token); err != nil || id != 42 || token != "token" {
				t.Fatalf("saved reference: %d/%s, %v", id, token, err)
			}
		})
	}
}
