package collector

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestAppLifecycle(t *testing.T) {
	for _, override := range []bool{false, true} {
		name := "default_download_directory"
		if override {
			name = "explicit_download_directory"
		}
		t.Run(name, func(t *testing.T) {
			published := time.Now().UTC().Truncate(time.Second)
			body := []byte(fmt.Sprintf(`<feed xmlns="http://www.w3.org/2005/Atom"><entry>
				<title>Feed title</title><updated>%s</updated><link href="https://example.test/g/42/token/"/>
				<content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Tags: language:english</p></div></content>
				</entry></feed>`, published.Format(time.RFC3339)))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_, _ = io.WriteString(w, `{"gmetadata":[{"gid":42,"token":"token","title":"Collected title"}]}`)
					return
				}
				_, _ = w.Write(body)
			}))
			defer upstream.Close()
			db := pgtest.Open(t, storage.Migrations())
			t.Setenv("TANA_COLLECTOR_DATABASE_URL", db.Config().ConnString())
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
				if err := app.db.QueryRow(t.Context(), "SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL").Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n == 1 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("collector did not process feed")
				}
				time.Sleep(time.Millisecond)
			}
			provisional, err := app.Catalog.List(t.Context(), collectorapi.CatalogOptions{Query: "language:english", PageSize: 24})
			if err != nil || len(provisional.Items) != 1 {
				t.Fatalf("feed metadata absent from catalog: %+v, %v", provisional, err)
			}
			item := provisional.Items[0]
			if item.Title != "Feed title" || item.MetadataSource != "feed" || item.MetadataState != "waiting" ||
				item.PostedAt == nil || !item.PostedAt.Equal(published) || item.PageCount != nil ||
				item.FetchEligibleAt == nil || !item.FetchEligibleAt.Equal(published.Add(12*time.Hour)) {
				t.Fatalf("unexpected provisional metadata: %+v", item)
			}
			if _, err := app.Metadata.Get(t.Context(), 42); !errors.Is(err, pgx.ErrNoRows) {
				t.Fatalf("feed description treated as collected metadata: %v", err)
			}
			lookup, err := app.Metadata.Lookup(t.Context(), []int64{42})
			if err != nil || len(lookup.Galleries) != 0 || len(lookup.PendingIDs) != 1 {
				t.Fatalf("local enrichment should wait for collected metadata: %+v, %v", lookup, err)
			}
			if _, err := app.Metadata.RequestFetch(t.Context(), []panda.GalleryRef{{ID: 42, Token: "token"}}); err != nil {
				t.Fatal(err)
			}
			deadline = time.Now().Add(5 * time.Second)
			for {
				collected, err := app.Metadata.Get(t.Context(), 42)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					t.Fatal(err)
				}
				if err == nil && collected.Metadata.Title == "Collected title" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("explicit request did not bypass feed delay")
				}
				time.Sleep(time.Millisecond)
			}
			catalog, err := app.Catalog.List(t.Context(), collectorapi.CatalogOptions{Query: "title:collected", PageSize: 24})
			if err != nil || len(catalog.Items) != 1 || catalog.Items[0].URL != upstream.URL+"/g/42/token/" || catalog.Items[0].MetadataSource != "collected" {
				t.Fatalf("collected feed metadata absent from catalog: %+v, %v", catalog, err)
			}
			staleTags, err := app.Catalog.List(t.Context(), collectorapi.CatalogOptions{Query: "language:english", PageSize: 24})
			if err != nil || len(staleTags.Items) != 0 {
				t.Fatalf("collected metadata did not replace feed tags: %+v, %v", staleTags, err)
			}
			if err := app.Close(); err != nil {
				t.Fatal(err)
			}
			var saved []byte
			if err := db.QueryRow(t.Context(), "SELECT body FROM raw_feeds").Scan(&saved); err != nil || len(saved) != 0 {
				t.Fatalf("processed raw feed retained after close: %q, %v", saved, err)
			}
			var id int64
			var token string
			if err := db.QueryRow(t.Context(), "SELECT gallery_id, token FROM gallery_refs").Scan(&id, &token); err != nil || id != 42 || token != "token" {
				t.Fatalf("saved reference: %d/%s, %v", id, token, err)
			}
		})
	}
}
