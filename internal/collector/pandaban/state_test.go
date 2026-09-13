package pandaban_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestBanScopesAreIndependentAndSurviveReopen(t *testing.T) {
	for _, status := range []int{200, 403} {
		for _, first := range []string{"metadata", "favorites", "archive"} {
			t.Run(fmt.Sprintf("%d/%s", status, first), func(t *testing.T) {
				dir := t.TempDir()
				db, _, err := storage.Open(t.Context(), dir)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { db.Close() }()
				var calls atomic.Int64
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					minutes := 30
					if r.URL.Path == "/metadata" {
						minutes = 60
					}
					w.WriteHeader(status)
					fmt.Fprintf(w, "Your IP is temporarily banned for excessive pageloads. The ban expires in %d minutes", minutes)
				}))
				defer upstream.Close()
				main, authenticated := pandaban.New(db), pandaban.NewAuthenticated(db)
				cfg := panda.AuthenticatedConfig{
					ArchiverURL: upstream.URL + "/archive", FavoritesURL: upstream.URL + "/favorites",
					Cookies: map[string]string{"ipb_member_id": "42", "ipb_pass_hash": "hash", "sp": "3"},
				}
				favorites, err := panda.NewAuthenticatedClient(cfg, &http.Client{Transport: panda.BanTransport(authenticated, nil)})
				if err != nil {
					t.Fatal(err)
				}
				metadata, err := panda.NewClient(upstream.URL+"/metadata", &http.Client{Transport: panda.BanTransport(main, nil)})
				if err != nil {
					t.Fatal(err)
				}
				ref := panda.GalleryRef{ID: 1, Token: "123456789a"}
				requests := map[string]func() error{
					"metadata":  func() error { _, err := metadata.GetMetadata(t.Context(), []panda.GalleryRef{ref}); return err },
					"favorites": func() error { _, err := favorites.GetFavoritesPage(t.Context(), 2, ""); return err },
					"archive":   func() error { _, err := favorites.GetArchiveURL(t.Context(), ref); return err },
				}
				assertBan := func(request string, wantCalls int64) {
					t.Helper()
					var ban *panda.BanError
					if err := requests[request](); !errors.As(err, &ban) || calls.Load() != wantCalls {
						t.Fatalf("%s: error=%v calls=%d want %d", request, err, calls.Load(), wantCalls)
					}
				}
				assertBan(first, 1)
				other, blocked, unbanned := "metadata", "archive", main
				if first == "metadata" {
					other, blocked, unbanned = "favorites", "metadata", authenticated
				} else if first == "archive" {
					blocked = "favorites"
				}
				if until, err := unbanned.Until(t.Context()); err != nil || until.After(time.Now()) {
					t.Fatalf("ban crossed request groups: %v, %v", until, err)
				}
				assertBan(blocked, 1)
				assertBan(other, 2)
				assertBan("archive", 2)
				assertBan("metadata", 2)

				mainUntil, err := main.Until(t.Context())
				if err != nil || time.Until(mainUntil) < time.Hour {
					t.Fatalf("main deadline %v, %v", mainUntil, err)
				}
				authenticatedUntil, err := authenticated.Until(t.Context())
				if err != nil || time.Until(authenticatedUntil) < 30*time.Minute || !authenticatedUntil.Before(mainUntil) {
					t.Fatalf("authenticated deadline %v, %v", authenticatedUntil, err)
				}
				// Unrelated metadata success and shorter bans cannot clear either group.
				if _, err := db.Exec(`UPDATE metadata_retry SET failures = 0, next_attempt_at = 0`); err != nil {
					t.Fatal(err)
				}
				for _, state := range []*pandaban.State{main, authenticated} {
					if err := state.Extend(t.Context(), time.Now().Add(time.Minute)); err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
				db, _, err = storage.Open(t.Context(), dir)
				if err != nil {
					t.Fatal(err)
				}
				main, authenticated = pandaban.New(db), pandaban.NewAuthenticated(db)
				for state, want := range map[*pandaban.State]time.Time{main: mainUntil, authenticated: authenticatedUntil} {
					if restored, err := state.Until(t.Context()); err != nil || restored.UnixMilli() != want.UnixMilli() {
						t.Fatalf("ban not retained: %v want %v, %v", restored, want, err)
					}
				}
				metadata, err = panda.NewClient(upstream.URL+"/metadata", &http.Client{Transport: panda.BanTransport(main, nil)})
				if err != nil {
					t.Fatal(err)
				}
				// A new host, account, and cookies still share the authenticated ban.
				cfg.FavoritesURL, cfg.ArchiverURL = "https://changed.test/favorites", "https://changed.test/archive"
				cfg.AccountKey, cfg.Cookies = "99", map[string]string{"ipb_member_id": "99", "ipb_pass_hash": "changed"}
				favorites, err = panda.NewAuthenticatedClient(cfg, &http.Client{Transport: panda.BanTransport(authenticated, nil)})
				if err != nil {
					t.Fatal(err)
				}
				assertBan("metadata", 2)
				assertBan("favorites", 2)
				assertBan("archive", 2)
			})
		}
	}
}
