package panda

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

//go:embed testdata/favorites.html
var favoriteSample string

// Keep the fixture readable while exercising full pages and boundary cases.
func favoriteHTML(count int, next string) string {
	before, rest, _ := strings.Cut(favoriteSample, "<tbody>")
	rows, after, _ := strings.Cut(rest, "</tbody>")
	templates := strings.SplitAfter(strings.TrimSpace(rows), "</tr>")
	var b strings.Builder
	for i := range count {
		templateID := i%2 + 1
		row := strings.NewReplacer(
			fmt.Sprintf("/g/%d/", templateID), fmt.Sprintf("/g/%d/", i+1),
			fmt.Sprintf(`id="posted_%d"`, templateID), fmt.Sprintf(`id="posted_%d"`, i+1),
			fmt.Sprintf(`id="favnote_%d"`, templateID), fmt.Sprintf(`id="favnote_%d"`, i+1),
			fmt.Sprintf("2000-01-03 12:%02d", 60-templateID), fmt.Sprintf("2000-01-03 12:%02d", 59-i/2),
		).Replace(templates[i%2])
		b.WriteString(row)
	}
	body := before + "<tbody>" + b.String() + "</tbody>" + after
	if count == 0 {
		body = strings.Replace(body, "</table>", "</table><p>No hits found</p>", 1)
	}
	navigation := `<span id="unext">Next &gt;</span>`
	if next != "" {
		navigation = fmt.Sprintf(`<a id="unext" href="%s">Next &gt;</a>`, next)
	}
	return strings.Replace(body, `<a id="unext" href="https://example.test/favorites.php?favcat=2&amp;next=100">Next &gt;</a>`, navigation, 1)
}

func TestFavoritesParser(t *testing.T) {
	body := favoriteHTML(100, `/favorites.php?favcat=2&amp;next=100`)
	page, err := ParseFavoritesPage([]byte(body), 2)
	if err != nil || page.CategoryName != "Reading & later" || len(page.Entries) != 100 || page.Next != "/favorites.php?favcat=2&next=100" {
		t.Fatalf("page: %+v, %v", page, err)
	}
	if page.Entries[0].GalleryRef != (GalleryRef{ID: 1, Token: "123456789a"}) || page.Entries[0].AddedAt.Format("2006-01-02 15:04") != "2000-01-03 12:59" {
		t.Fatalf("entry: %+v", page.Entries[0])
	}
	page, err = ParseFavoritesPage([]byte(favoriteHTML(0, "")), 2)
	if err != nil || len(page.Entries) != 0 {
		t.Fatalf("empty category: %+v, %v", page, err)
	}
	for name, body := range map[string]string{
		"login":                `<html>Please log in</html>`,
		"unexpected empty":     strings.Replace(favoriteHTML(0, ""), "No hits found", "Unexpected response", 1),
		"short non-final page": favoriteHTML(25, "/favorites.php?next=25"),
		"timestamp missing":    strings.Replace(favoriteHTML(1, ""), "2000-01-03 12:59", "unknown", 1),
		"wrong ordering":       strings.Replace(favoriteHTML(3, ""), "2000-01-03 12:58", "2000-01-03 13:00", 1),
		"wrong profile":        strings.Replace(favoriteHTML(1, ""), "<body>", `<body><select name="f_perpage"><option value="25" selected>25</option></select>`, 1),
		"wrong sort":           strings.Replace(favoriteHTML(1, ""), "<body>", `<body><select name="f_sort"><option selected>Posted</option></select>`, 1),
		"truncated pagination": strings.Replace(favoriteHTML(100, ""), `<span id="unext">Next &gt;</span>`, "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFavoritesPage([]byte(body), 2); !errors.Is(err, ErrFavoritesPage) {
				t.Fatalf("accepted invalid response: %v", err)
			}
		})
	}
}

func TestFavoritesClientCookiesAndPagination(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		for name, want := range map[string]string{"ipb_member_id": "42", "ipb_pass_hash": "hash", "sp": "3", "igneous": "extra"} {
			cookie, err := r.Cookie(name)
			if err != nil || cookie.Value != want {
				t.Errorf("cookie %s not seeded", name)
			}
		}
		if r.UserAgent() != "Tana" || r.URL.Path != "/account/saved-items" || r.URL.Query().Get("favcat") != "2" || r.URL.Query().Get("view") != "table" {
			t.Errorf("request: %s, agent %s", r.URL, r.UserAgent())
		}
		if requests == 1 {
			http.SetCookie(w, &http.Cookie{Name: "server_session", Value: "updated", Path: "/"})
			fmt.Fprint(w, favoriteHTML(100, "?next=100"))
		} else {
			cookie, err := r.Cookie("server_session")
			if err != nil || cookie.Value != "updated" {
				t.Error("server cookie not carried to next page")
			}
			fmt.Fprint(w, favoriteHTML(1, ""))
		}
	}))
	defer upstream.Close()
	client, err := NewFavoritesClient(FavoritesConfig{URL: upstream.URL + "/account/saved-items?view=table", Cookies: map[string]string{"ipb_member_id": "42", "ipb_pass_hash": "hash", "sp": "3", "igneous": "extra"}}, upstream.Client())
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.GetFavoritesPage(t.Context(), 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Next != upstream.URL+"/account/saved-items?favcat=2&next=100&view=table" {
		t.Fatalf("resolved next page: %s", page.Next)
	}
	if _, err := client.GetFavoritesPage(t.Context(), 2, page.Next); err != nil {
		t.Fatal(err)
	}
	for _, next := range []string{"https://elsewhere.test/account/saved-items", upstream.URL + "/account/saved-items?favcat=3", upstream.URL + "/login.php"} {
		if _, err := client.GetFavoritesPage(t.Context(), 2, next); !errors.Is(err, ErrFavoritesPage) {
			t.Fatalf("accepted pagination %s: %v", next, err)
		}
	}
	if requests != 2 {
		t.Fatalf("unexpected upstream requests: %d", requests)
	}
}

func TestFavoritesConfig(t *testing.T) {
	env := map[string]string{"PANDA_FAVORITES_URL": " https://EXAMPLE.test/account/saved-items?view=table ", "PANDA_FAVORITES_COOKIES": `{"ipb_member_id":"42","custom":"a=b"}`}
	load := func() (FavoritesConfig, error) {
		return LoadFavoritesConfig(func(key string) string { return env[key] })
	}
	cfg, err := load()
	if err != nil || cfg.URL != "https://example.test/account/saved-items?view=table" || cfg.Origin() != "https://example.test" || cfg.AccountKey != "42" || cfg.Cookies["custom"] != "a=b" {
		t.Fatalf("config: %+v, %v", cfg, err)
	}
	for _, key := range []string{"PANDA_FAVORITES_URL", "PANDA_FAVORITES_COOKIES"} {
		saved := env[key]
		env[key] = ""
		if _, err := load(); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("missing %s: %v", key, err)
		}
		env[key] = saved
	}
	savedURL := env["PANDA_FAVORITES_URL"]
	for _, value := range []string{"/account/saved-items", "ftp://example.test/saved", "https://user:pass@example.test/saved", "https://example.test/saved#section", "https://example.test/saved?view=%zz"} {
		env["PANDA_FAVORITES_URL"] = value
		if _, err := load(); err == nil || !strings.Contains(err.Error(), "PANDA_FAVORITES_URL") {
			t.Fatalf("invalid favorites URL accepted: %v", err)
		}
	}
	env["PANDA_FAVORITES_URL"] = savedURL
	env["PANDA_FAVORITES_ACCOUNT_KEY"] = "personal"
	for _, value := range []string{`[]`, `null`, `{"x":null}`, `{"invalid name":"x"}`, `{"x":"a;b"}`, `{"x":1}`} {
		env["PANDA_FAVORITES_COOKIES"] = value
		if _, err := load(); err == nil || !strings.Contains(err.Error(), "PANDA_FAVORITES_COOKIES") {
			t.Fatalf("accepted cookies: %s", value)
		}
	}
}

func TestFavoritesAccountKeyFallback(t *testing.T) {
	for _, tc := range []struct{ name, key, cookies, want string }{
		{"member cookie default", "", `{"ipb_member_id":"42"}`, "42"},
		{"explicit key", " personal ", `{"ipb_member_id":"42"}`, "personal"},
		{"opaque cookies", "personal", `{"session":"custom"}`, "personal"},
		{"no mandatory authentication cookies", "personal", `{}`, "personal"},
		{"missing identity", "", `{"session":"custom"}`, ""},
		{"empty member cookie", "", `{"ipb_member_id":""}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"PANDA_FAVORITES_URL": "https://example.test/saved", "PANDA_FAVORITES_ACCOUNT_KEY": tc.key, "PANDA_FAVORITES_COOKIES": tc.cookies}
			cfg, err := LoadFavoritesConfig(func(key string) string { return env[key] })
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), "ipb_member_id") {
					t.Fatalf("missing account identity accepted: %v", err)
				}
			} else if err != nil || cfg.AccountKey != tc.want {
				t.Fatalf("account key=%s, error=%v", cfg.AccountKey, err)
			}
		})
	}
}

func TestBanTextDetection(t *testing.T) {
	at := time.Unix(1000, 0)
	for _, tc := range []struct {
		body  string
		delay time.Duration
	}{
		{"Your IP address has been temporarily banned for excessive pageloads\nwhich indicates that you are using automated mirroring/harvesting\nsoftware. The ban expires in 58 minutes and 39 seconds", 58*time.Minute + 44*time.Second},
		{"IP TEMPORARILY BANNED: excessive pageloads. Ban expires in 1 hour, 2 minutes and 3 seconds", time.Hour + 2*time.Minute + 8*time.Second},
		{"Your IP has been banned for excessive pageloads. Try later.", 24 * time.Hour},
		{"<html>Your IP has been temporarily banned for excessive pageloads</html>", 0},
		{"Normal plain text", 0},
	} {
		ban := DetectBan([]byte(tc.body), at)
		if tc.delay == 0 {
			if ban != nil {
				t.Fatalf("false ban: %s", tc.body)
			}
			continue
		}
		if ban == nil || ban.Until.Sub(at) != tc.delay {
			t.Fatalf("ban=%v, want %v", ban, tc.delay)
		}
	}
}
