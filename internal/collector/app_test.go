package collector

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
)

func TestAppLifecycle(t *testing.T) {
	body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><link href="https://example.test/g/42/token/"/></entry></feed>`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	dir := t.TempDir()
	t.Setenv("TANA_COLLECTOR_DATA_DIR", dir)
	t.Setenv("PANDA_FEED_URL", upstream.URL)
	t.Setenv("PANDA_FEED_INTERVAL", "1h")
	t.Setenv("PANDA_FEED_RETRY_DELAY", "1m")
	app, err := New(t.Context(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	w := httptest.NewRecorder()
	app.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("health: %d", w.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		if err := app.db.QueryRow("SELECT count(*) FROM raw_feeds WHERE processed_at IS NOT NULL").Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("collector did not capture and process feed")
		}
		time.Sleep(time.Millisecond)
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
}

func TestMissingFeedURLPreventsStartup(t *testing.T) {
	t.Setenv("PANDA_FEED_URL", "")
	app, err := New(t.Context(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	if app != nil {
		_ = app.Close()
		t.Fatal("started collector without a feed URL")
	}
	if err == nil {
		t.Fatal("missing feed URL accepted")
	}
}
