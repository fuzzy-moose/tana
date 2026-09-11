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

func TestBanBlocksBothClientsAndSurvivesReopen(t *testing.T) {
	for _, status := range []int{200, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			dir := t.TempDir()
			db, _, err := storage.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { db.Close() }()
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(status)
				fmt.Fprint(w, "Your IP is temporarily banned for excessive pageloads. The ban expires in 58 minutes and 39 seconds")
			}))
			defer upstream.Close()
			state := pandaban.New(db)
			httpClient := &http.Client{Transport: panda.BanTransport(state, nil)}
			favorites, err := panda.NewAuthenticatedClient(panda.AuthenticatedConfig{ArchiverURL: upstream.URL + "/account/prepare-archive", FavoritesURL: upstream.URL, Cookies: map[string]string{"ipb_member_id": "42", "ipb_pass_hash": "hash", "sp": "3"}}, httpClient)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := panda.NewClient(upstream.URL, &http.Client{Transport: panda.BanTransport(state, nil)})
			if err != nil {
				t.Fatal(err)
			}
			_, err = favorites.GetFavoritesPage(t.Context(), 2, "")
			var ban *panda.BanError
			if !errors.As(err, &ban) {
				t.Fatalf("ban not recognized before status classification: %v", err)
			}
			_, err = metadata.GetMetadata(t.Context(), []panda.GalleryRef{{ID: 1, Token: "123456789a"}})
			if !errors.As(err, &ban) || calls.Load() != 1 {
				t.Fatalf("metadata bypassed ban: %v, calls=%d", err, calls.Load())
			}
			_, err = favorites.GetArchiveURL(t.Context(), panda.GalleryRef{ID: 1, Token: "123456789a"})
			if !errors.As(err, &ban) || calls.Load() != 1 {
				t.Fatalf("archive preparation bypassed ban: %v, calls=%d", err, calls.Load())
			}
			until, err := state.Until(t.Context())
			if err != nil || time.Until(until) < 58*time.Minute {
				t.Fatalf("deadline %v, %v", until, err)
			}
			// An unrelated metadata success and a shorter ban cannot clear it.
			if _, err := db.Exec(`UPDATE metadata_retry SET failures = 0, next_attempt_at = 0`); err != nil {
				t.Fatal(err)
			}
			if err := state.Extend(t.Context(), time.Now().Add(time.Minute)); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, _, err = storage.Open(t.Context(), dir)
			if err != nil {
				t.Fatal(err)
			}
			state = pandaban.New(db)
			restored, err := state.Until(t.Context())
			if err != nil || restored.UnixMilli() != until.UnixMilli() {
				t.Fatalf("ban not retained: %v want %v, %v", restored, until, err)
			}
			metadata, err = panda.NewClient(upstream.URL, &http.Client{Transport: panda.BanTransport(state, nil)})
			if err != nil {
				t.Fatal(err)
			}
			_, err = metadata.GetMetadata(t.Context(), []panda.GalleryRef{{ID: 1, Token: "123456789a"}})
			if !errors.As(err, &ban) || calls.Load() != 1 {
				t.Fatalf("restart bypassed ban: %v", err)
			}
			// Feeds intentionally use their own transport.
			resp, err := upstream.Client().Get(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if calls.Load() != 2 {
				t.Fatal("separate feed transport was blocked")
			}
		})
	}
}
