package httpapi

import (
	"encoding/json"
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
	var password, name string
	if err := db.QueryRow(`SELECT name, password FROM metadata_proxy_channels WHERE id = ?`, id).Scan(&name, &password); err != nil || name != "Renamed" || password != "private-password" {
		t.Fatal("configuration did not persist")
	}
	request("PUT", "/channels/missing", input, 404)
	request("POST", "/channels", `{"name":"bad","proxy_url":"ftp://proxy.invalid"}`, 400)
	request("DELETE", "/channels/"+id, "", 200)
	request("DELETE", "/channels/missing", "", 404)
}
