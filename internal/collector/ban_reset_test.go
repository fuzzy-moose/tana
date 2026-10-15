package collector

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestBanResetResumesWaitingWork(t *testing.T) {
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	if _, err := w.Create("page.jpg"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	const banText = "Your IP has been temporarily banned for excessive pageloads. The ban expires in 1 day"
	var metadataRequests, archiveRequests atomic.Int64
	originalTransport := http.DefaultTransport
	transport := originalTransport.(*http.Transport).Clone()
	transport.RegisterProtocol("https", banTestTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Hostname() + r.URL.Path {
		case "panda.test/feed":
			body = `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`
		case "panda.test/api":
			body = `{"gmetadata":[{"gid":1,"token":"token","title":"Gallery"}]}`
			if metadataRequests.Add(1) == 1 {
				body = banText
			}
		case "panda.test/archive":
			body = `<a href="https://node.hath.network/archive">Download</a>`
			if archiveRequests.Add(1) == 1 {
				body = banText
			}
		case "node.hath.network/archive":
			body = archive.String()
		default:
			return nil, fmt.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	http.DefaultTransport = transport
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
		transport.CloseIdleConnections()
	})
	values := map[string]string{
		"TANA_COLLECTOR_DATA_DIR": t.TempDir(), "TANA_COLLECTOR_DOWNLOAD_DIR": t.TempDir(), "TANA_COLLECTOR_API_TOKEN": "test-token",
		"PANDA_FEED_URL": "https://panda.test/feed", "PANDA_API_URL": "https://panda.test/api",
		"PANDA_FAVORITES_URL": "https://panda.test/favorites", "PANDA_ARCHIVER_URL": "https://panda.test/archive",
		"PANDA_FAVORITES_COOKIES": `{"ipb_member_id":"test"}`,
	}
	cfg, err := LoadConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(t.Context(), cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })

	ref := panda.GalleryRef{ID: 1, Token: "token"}
	fetch, err := app.Metadata.RequestFetch(t.Context(), []panda.GalleryRef{ref})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.Downloads.Submit(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "both groups banned", func() bool { return metadataRequests.Load() == 1 && archiveRequests.Load() == 1 })
	waitFor(t, "ban recorded", func() bool {
		job, err := app.Downloads.Get(t.Context(), 1)
		return err == nil && job.Error == "panda_banned"
	})

	if err := app.Bans.Reset(t.Context(), "unknown"); err != ErrUnknownBanGroup {
		t.Fatalf("unknown group: %v", err)
	}
	for _, group := range []string{"main", "authenticated"} {
		if err := app.Bans.Reset(t.Context(), group); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, "metadata fetched", func() bool {
		job, err := app.Metadata.GetFetchJob(t.Context(), fetch.ID)
		return err == nil && job.CompletedAt != nil
	})
	waitFor(t, "archive downloaded", func() bool {
		job, err := app.Downloads.Get(t.Context(), 1)
		return err == nil && job.State == "completed"
	})
	if until, err := app.Ban.Until(t.Context()); err != nil || until.After(time.Now()) {
		t.Fatalf("main ban remained: %v, %v", until, err)
	}
}

func waitFor(t *testing.T, name string, done func() bool) {
	t.Helper()
	// Authenticated requests are paced 10 seconds apart.
	deadline := time.Now().Add(20 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", name)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
