package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/metadataproxy"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestLocalMetadataProxyConfiguration(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := slog.New(slog.DiscardHandler)
	// Collection stays disabled here; this test exercises the real settings
	// path through both APIs, authentication, storage and password redaction.
	proxies, err := metadataproxy.New(t.Context(), db, nil, panda.Config{APIURL: "https://panda.invalid/api", RateInterval: time.Second}, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer proxies.Close()
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, MetadataProxies: proxies, APIToken: "collector-secret"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "Bearer collector-secret" {
			t.Error("wrong collector credentials")
		}
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: logger, Collector: client})
	request := func(method, suffix, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "/api/collector/metadata/proxies"+suffix, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Cookie", "browser-secret=hidden")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s", method, suffix, w.Code, w.Body)
		}
		if strings.Contains(w.Body.String(), "private-password") || strings.Contains(w.Body.String(), "collector-secret") || strings.Contains(w.Body.String(), `"password":`) {
			t.Fatal("credentials exposed")
		}
		return w
	}
	request("GET", "", "", 200)
	input := `{"name":"Proxy A","proxy_url":"http://proxy.invalid:8080","username":"user","password":"private-password","user_agent":"Browser","enabled":false}`
	created := request("POST", "/channels", input, 200)
	var result collectorapi.MetadataProxyStatus
	if err := json.Unmarshal(created.Body.Bytes(), &result); err != nil || len(result.Channels) != 1 || !result.Channels[0].HasPassword {
		t.Fatalf("create: %+v, %v", result, err)
	}
	id := result.Channels[0].ID
	request("POST", "/channels", input, 409)
	request("PUT", "/channels/"+id, `{"name":"Renamed","proxy_url":"http://proxy.invalid:8080","username":"user","user_agent":"Another Browser","enabled":false}`, 200)
	request("PUT", "", `{"enabled":true}`, 200)
	request("PUT", "", `{"enabled":false}`, 200)
	request("GET", "", "", 200)
	for _, cleanup := range []bool{true, false} {
		request("PUT", "", fmt.Sprintf(`{"enabled":false,"auto_remove_inactive":%t}`, cleanup), 200)
		if err := json.Unmarshal(request("GET", "", "", 200).Body.Bytes(), &result); err != nil || result.AutoRemoveInactive != cleanup || result.Enabled {
			t.Fatalf("cleanup setting: %+v, %v", result, err)
		}
	}
	var password, name string
	if err := db.QueryRow(`SELECT name, password FROM metadata_proxy_channels WHERE id = ?`, id).Scan(&name, &password); err != nil || name != "Renamed" || password != "private-password" {
		t.Fatal("configuration did not persist")
	}
	request("PUT", "/channels/missing", input, 404)
	request("POST", "/channels", `{"name":"bad","proxy_url":"ftp://proxy.invalid"}`, 400)

	t.Run("imports lists without changing existing channels", func(t *testing.T) {
		until := time.Now().Add(time.Hour).UnixMilli()
		if _, err := db.Exec(`INSERT INTO metadata_proxy_bans (endpoint, until_at) VALUES (?, ?)`, "http://other.invalid:443", until); err != nil {
			t.Fatal(err)
		}
		importList := func(proxies string, want int) *httptest.ResponseRecorder {
			t.Helper()
			body, err := json.Marshal(collectorapi.MetadataProxyImportInput{Proxies: proxies, Protocol: "http", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			return request("POST", "/import", string(body), want)
		}
		const list = "proxy.invalid:8080\nnew.invalid:1080\nnew.invalid:1080\nother.invalid:443"
		var imported collectorapi.MetadataProxyImportResult
		if err := json.Unmarshal(importList(list, 200).Body.Bytes(), &imported); err != nil {
			t.Fatal(err)
		}
		if imported.Added != 2 || imported.Duplicates != 2 || imported.Status.Enabled || len(imported.Status.Channels) != 3 {
			t.Fatalf("import result: %+v", imported)
		}
		old, fresh, banned := imported.Status.Channels[0], imported.Status.Channels[1], imported.Status.Channels[2]
		if old.Name != "Renamed" || old.Enabled || old.UserAgent != "Another Browser" || !old.HasPassword {
			t.Fatalf("existing settings changed: %+v", old)
		}
		if fresh.ProxyURL != "http://new.invalid:1080" || fresh.Username != "" || fresh.HasPassword || !fresh.Enabled || fresh.UserAgent != imported.Status.DefaultUserAgent {
			t.Fatalf("new channel: %+v", fresh)
		}
		if banned.BanUntil == nil || banned.BanUntil.UnixMilli() != until {
			t.Fatalf("ban lost: %+v", banned)
		}
		if err := json.Unmarshal(importList(list, 200).Body.Bytes(), &imported); err != nil || imported.Added != 0 || imported.Duplicates != 4 {
			t.Fatalf("repeat import: %+v, %v", imported, err)
		}
		for _, failure := range []struct {
			proxies, code string
			status        int
		}{
			{"unsaved.invalid:8080\nnot a proxy", "proxy_list_invalid", 400},
			{strings.Repeat("x", (1<<20)+1), "proxy_list_too_large", 413},
		} {
			if w := importList(failure.proxies, failure.status); !strings.Contains(w.Body.String(), failure.code) {
				t.Fatalf("wrong error: %s", w.Body)
			}
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM metadata_proxy_channels`).Scan(&count); err != nil || count != 3 {
			t.Fatalf("failed import changed channels: %d, %v", count, err)
		}
		// Public lists can exceed the ordinary JSON request and response bounds.
		var large strings.Builder
		for i := range 3000 {
			fmt.Fprintf(&large, "proxy-%d.invalid:8080\n", i)
		}
		if err := json.Unmarshal(importList(large.String(), 200).Body.Bytes(), &imported); err != nil || imported.Added != 3000 || len(imported.Status.Channels) != 3003 {
			t.Fatalf("large paste: added=%d channels=%d, %v", imported.Added, len(imported.Status.Channels), err)
		}
		request("GET", "", "", 200)
	})
	request("DELETE", "/channels/"+id, "", 200)
	request("DELETE", "/channels/missing", "", 404)
}
