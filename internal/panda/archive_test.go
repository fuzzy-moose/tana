package panda

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArchivePreparationSharesFavoritesSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/account/saved-items" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "refreshed", Path: "/"})
			fmt.Fprint(w, favoriteHTML(1, ""))
			return
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value != "refreshed" {
			t.Error("archive request did not share updated favorites session")
		}
		if r.Method != "POST" || r.URL.Path != "/account/prepare-archive" || r.URL.Query().Get("gid") != "42" || r.URL.Query().Get("token") != "a&b" || r.URL.Query().Has("or") {
			t.Errorf("unexpected archive request: %s %s", r.Method, r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.PostForm.Get("dltype") != "org" || r.PostForm.Get("dlcheck") != "Download Original Archive" {
			t.Errorf("form: %v", r.PostForm)
		}
		fmt.Fprint(w, `<a href="https://node.hath.network/archive/path?x=1&amp;y=2">Download</a>`)
	}))
	defer upstream.Close()
	client, err := NewAuthenticatedClient(AuthenticatedConfig{ArchiverURL: upstream.URL + "/account/prepare-archive", FavoritesURL: upstream.URL + "/account/saved-items", Cookies: map[string]string{"session": "seed"}}, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetFavoritesPage(t.Context(), 2, ""); err != nil {
		t.Fatal(err)
	}
	address, err := client.GetArchiveURL(t.Context(), GalleryRef{ID: 42, Token: "a&b"})
	if err != nil || address != "https://node.hath.network/archive/path?start=1&x=1&y=2" {
		t.Fatalf("archive: %s, %v", address, err)
	}
}

func TestArchivePreparationRejectsErrorsAndUntrustedLinks(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"login", `<html>Log in</html>`, 200, ErrArchivePage},
		{"invalid key", `Invalid archiver key`, 200, ErrArchivePage},
		{"untrusted URL", `<a href="https://node.hath.network.evil.test/archive">Download</a>`, 200, ErrArchivePage},
		{"ban", `Your IP is temporarily banned for excessive pageloads. Ban expires in 1 hour`, 200, &BanError{}},
		{"unavailable", `Unavailable`, 404, &HTTPError{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer upstream.Close()
			client, err := NewAuthenticatedClient(AuthenticatedConfig{ArchiverURL: upstream.URL + "/account/prepare-archive", FavoritesURL: upstream.URL + "/favorites"}, upstream.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.GetArchiveURL(t.Context(), GalleryRef{ID: 42, Token: "token"})
			switch tc.want.(type) {
			case *BanError:
				if _, ok := errors.AsType[*BanError](err); !ok {
					t.Fatalf("expected ban, got %v", err)
				}
			case *HTTPError:
				if e, ok := errors.AsType[*HTTPError](err); !ok || e.StatusCode != 404 {
					t.Fatalf("expected 404, got %v", err)
				}
			default:
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v", err)
				}
			}
		})
	}
}
