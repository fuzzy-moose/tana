package collector

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type banTestTransport func(*http.Request) (*http.Response, error)

func (f banTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestArchiveTransfersStayOutsidePandaBans(t *testing.T) {
	var archive bytes.Buffer
	w := zip.NewWriter(&archive)
	entry, err := w.Create("page.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, "image contents"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		body string
	}{
		{"valid_archive", archive.String()},
		{"ban_text_is_invalid_archive", "Your IP has been temporarily banned for excessive pageloads. Ban expires in 1 day."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var authenticatedBan *pandaban.State
			until := time.Now().Add(time.Hour).Truncate(time.Millisecond)
			originalTransport := http.DefaultTransport
			transport := originalTransport.(*http.Transport).Clone()
			transport.RegisterProtocol("https", banTestTransport(func(r *http.Request) (*http.Response, error) {
				body := ""
				switch r.URL.Hostname() {
				case "panda.test":
					switch r.URL.Path {
					case "/feed":
						body = `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`
					case "/archive":
						// A ban discovered after link preparation must not block the transfer.
						if err := authenticatedBan.Extend(r.Context(), until); err != nil {
							return nil, err
						}
						body = `<a href="https://node.hath.network/archive">Download</a>`
					default:
						return nil, fmt.Errorf("unexpected request: %s", r.URL)
					}
				case "node.hath.network":
					if r.Header.Get("Cookie") != "" {
						t.Error("authenticated cookies reached archive host")
					}
					body = tc.body
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
				"TANA_COLLECTOR_DATA_DIR": t.TempDir(), "TANA_COLLECTOR_API_TOKEN": "test-token",
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
			authenticatedBan = pandaban.NewAuthenticated(app.db)
			if err := app.Ban.Extend(t.Context(), until); err != nil {
				t.Fatal(err)
			}
			if _, err := app.Downloads.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				job, err := app.Downloads.Get(t.Context(), 1)
				if err != nil {
					t.Fatal(err)
				}
				if job.State == "completed" || job.Error != "" {
					if tc.name == "valid_archive" {
						if job.State != "completed" || job.SizeBytes != int64(archive.Len()) {
							t.Fatalf("bans prevented archive transfer: %+v", job)
						}
					} else if job.Error != "invalid_zip" || job.Failures != 1 || job.RetryAt == nil {
						t.Fatalf("transfer response did not use ordinary retry policy: %+v", job)
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("download did not finish: %+v", job)
				}
				time.Sleep(time.Millisecond)
			}
			for _, state := range []*pandaban.State{app.Ban, authenticatedBan} {
				got, err := state.Until(t.Context())
				if err != nil || !got.Equal(until) {
					t.Fatalf("archive transfer changed a Panda ban: %v, %v", got, err)
				}
			}
		})
	}
}
