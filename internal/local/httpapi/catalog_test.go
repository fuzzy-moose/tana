package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/local/catalogfilter"
	localstorage "github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestPandaCatalogThroughLocalProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer collector-secret" {
			t.Error("missing collector authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/catalog":
			if r.URL.Query().Get("q") != `p:"show z$"` || r.URL.Query().Get("cursor") != "next-token" ||
				r.URL.Query().Get("page_size") != "12" || r.URL.Query().Get("include_expunged") != "true" {
				t.Errorf("catalog query changed: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"items":[{"gallery_id":42,"title":"Show Z","thumbnail_url":"https://images.test/42.jpg","page_count":20,"posted_at":"2026-09-12T00:00:00Z","url":"https://panda.test/g/42/token/"}],"page_size":12,"previous_cursor":"previous-token"}`))
		case "/api/catalog/completions":
			if r.URL.Query().Get("q") != "p:show" || r.URL.Query().Get("cursor") != "6" {
				t.Errorf("completion query changed: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"start":0,"end":6,"items":[{"namespace":"parody","value":"show z","term":"parody:\"show z$\""}]}`))
		case "/api/feed/status", "/api/feed/refresh":
			if r.URL.Path == "/api/feed/refresh" {
				if r.Method != http.MethodPost {
					t.Errorf("refresh method: %s", r.Method)
				}
				w.WriteHeader(http.StatusAccepted)
			}
			_, _ = w.Write([]byte(`{"capture_active":true,"processing_pending":2,"continuity":"overlap","possible_gaps":0}`))
		default:
			t.Errorf("unexpected upstream request: %s", r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Collector: client})
	for _, tc := range []struct {
		method, path, body, want string
		code                     int
	}{
		{"GET", "/api/collector/catalog?q=p%3A%22show+z%24%22&cursor=next-token&page_size=12&include_expunged=true", "", `"gallery_id":42`, 200},
		{"GET", "/api/collector/catalog/completions?q=p%3Ashow&cursor=6", "", `"namespace":"parody"`, 200},
		{"GET", "/api/collector/feed/status", "", `"processing_pending":2`, 200},
		{"POST", "/api/collector/feed/refresh", `{}`, `"capture_active":true`, 202},
		{"GET", "/api/collector/catalog?page_size=0", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?page_size=101", "", "invalid_pagination", 400},
		{"GET", "/api/collector/catalog?include_expunged=garbage", "", "invalid_query", 400},
		{"GET", "/api/collector/catalog?unseen=garbage", "", "invalid_query", 400},
		{"GET", "/api/collector/catalog/completions?q=x&cursor=-1", "", "invalid_query", 400},
		{"POST", "/api/collector/feed/refresh", `{"unknown":true}`, "invalid", 400},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "collector-secret") {
				t.Fatalf("got %d %s", w.Code, w.Body)
			}
		})
	}
}

func TestCollectorCatalogForwardsUnseen(t *testing.T) {
	var unseen atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		unseen.Store(r.URL.Query()["unseen"])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"page_size":24}`))
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(&local.App{Logger: slog.New(slog.DiscardHandler), Collector: client})
	for path, want := range map[string]string{
		"/api/collector/catalog?unseen=true": "true",
		"/api/collector/catalog":             "false",
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if got, _ := unseen.Load().([]string); w.Code != http.StatusOK || !slices.Equal(got, []string{want}) {
			t.Fatalf("%s: %d %s, upstream unseen = %v", path, w.Code, w.Body, got)
		}
	}
}

func TestPandaCatalogProxyErrors(t *testing.T) {
	for _, tc := range []struct {
		upstream, want int
		body, code     string
	}{
		{400, 400, `{"error":"invalid_query"}`, "invalid_query"},
		{400, 400, `{"error":"invalid_category"}`, "invalid_category"},
		{400, 400, `{"error":"invalid_pagination"}`, "invalid_pagination"},
		{401, 502, `{"error":"secret details"}`, "collector_unauthorized"},
		{500, 502, `{"error":"secret details"}`, "collector_unavailable"},
	} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.upstream)
			_, _ = w.Write([]byte(tc.body))
		}))
		client, err := collectorapi.NewClient(upstream.URL, "collector-secret")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		HandleCollectorCatalog(client).ServeHTTP(w, httptest.NewRequest("GET", "/?q=unknown:value", nil))
		upstream.Close()
		if w.Code != tc.want || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "secret") {
			t.Fatalf("upstream %d: got %d %s", tc.upstream, w.Code, w.Body)
		}
	}
	for _, handler := range []http.Handler{HandleCollectorCatalog(nil), HandleCollectorCatalogCompletions(nil), HandleCollectorFeed(nil)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
		}
	}
}

func TestLocalRecordCatalogDisplays(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 3; id++ {
		if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: fmt.Sprintf("Gallery %d", id), Posted: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	var lastBody atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		lastBody.Store(string(body))
		r.Body = io.NopCloser(bytes.NewReader(body))
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/displays"
	if w := request(client, "POST", path, `{"gallery_ids":[3,1]}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("record: %d %s", w.Code, w.Body)
	}
	if got := lastBody.Load(); got != `{"gallery_ids":[3,1]}` {
		t.Fatalf("collector body without a score filter: %v", got)
	}
	w := request(client, "GET", "/api/collector/catalog", "")
	var page collectorapi.CatalogResult
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("catalog: %d %s, %v", w.Code, w.Body, err)
	}
	displayed := map[int64]bool{}
	for _, item := range page.Items {
		displayed[item.GalleryID] = item.Displayed
	}
	if !maps.Equal(displayed, map[int64]bool{1: true, 2: false, 3: true}) || !strings.Contains(w.Body.String(), `"displayed":true`) {
		t.Fatalf("displayed flags: %s", w.Body)
	}
	if w := request(client, "POST", path, `{"gallery_ids":[2],"score_filter":"hidden"}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("record hidden: %d %s", w.Code, w.Body)
	}
	var evidence []bool
	if err := db.QueryRow(t.Context(), "SELECT array_agg(evidence ORDER BY gallery_id) FROM panda_gallery_displays").Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(evidence, []bool{true, false, true}) {
		t.Fatalf("evidence by gallery 1, 2, 3: %v", evidence)
	}
	tooMany := make([]string, collectorapi.MaxCatalogDisplays+1)
	for i := range tooMany {
		tooMany[i] = strconv.Itoa(i + 1)
	}
	before := requests.Load()
	for _, body := range []string{`{"gallery_ids":[]}`, `{"gallery_ids":[` + strings.Join(tooMany, ",") + `]}`, `{"gallery_ids":[2,0]}`, `{"gallery_ids":[-1]}`, `{"gallery_ids":[2],"score_filter":"bogus"}`} {
		if w := request(client, "POST", path, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_batch") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if w := request(client, "POST", path, `{"gallery_ids":[1],"unknown":true}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_json") {
		t.Fatalf("unknown field: %d %s", w.Code, w.Body)
	}
	if requests.Load() != before {
		t.Fatal("invalid lists reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"gallery_ids":[2]}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "POST", path, `{"gallery_ids":[2]}`); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalCatalogCatchUp(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for id := int64(1); id <= 3; id++ {
		if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: fmt.Sprintf("Gallery %d", id), Posted: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/catch-up"
	if w := request(client, "POST", path, `{"gallery_id":2}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("catch-up: %d %s", w.Code, w.Body)
	}
	w := request(client, "GET", "/api/collector/catalog?unseen=true", "")
	var page collectorapi.CatalogResult
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("unseen catalog: %d %s, %v", w.Code, w.Body, err)
	}
	if len(page.Items) != 1 || page.Items[0].GalleryID != 3 {
		t.Fatalf("unseen after catch-up: %s", w.Body)
	}
	if w := request(client, "POST", path, `{"gallery_id":99}`); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "gallery_not_found") {
		t.Fatalf("unknown gallery: %d %s", w.Code, w.Body)
	}
	before := requests.Load()
	for _, body := range []string{`{"gallery_id":0}`, `{"gallery_id":-1}`, `{}`} {
		if w := request(client, "POST", path, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_gallery_reference") {
			t.Fatalf("%s: %d %s", body, w.Code, w.Body)
		}
	}
	if requests.Load() != before {
		t.Fatal("invalid gallery references reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"gallery_id":3}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "POST", path, `{"gallery_id":3}`); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalCatalogVerdicts(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'token')"); err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 2, Title: "Gallery 2", Posted: 2}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	listed := func() map[string]any {
		t.Helper()
		w := request(client, "GET", "/api/collector/catalog", "")
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK || len(page.Items) != 1 {
			t.Fatalf("catalog: %d %s, %v", w.Code, w.Body, err)
		}
		return page.Items[0]
	}
	const path = "/api/collector/catalog/verdicts/2"
	for _, verdict := range []string{"not_for_me", "interesting"} {
		if w := request(client, "PUT", path, `{"verdict":"`+verdict+`"}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
			t.Fatalf("put %s: %d %s", verdict, w.Code, w.Body)
		}
		if item := listed(); item["verdict"] != verdict || item["displayed"] != true {
			t.Fatalf("after %s: %v", verdict, item)
		}
	}
	if w := request(client, "DELETE", path, ""); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if item := listed(); item["verdict"] != nil || item["displayed"] != true {
		t.Fatalf("after delete: %v", item)
	}
	if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (3, 'token3')"); err != nil {
		t.Fatal(err)
	}
	tx, err = db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 3, Title: "Gallery 3", Posted: 3}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	for id, verdict := range map[string]string{"2": "interesting", "3": "not_for_me"} {
		if w := request(client, "PUT", "/api/collector/catalog/verdicts/"+id, `{"verdict":"`+verdict+`"}`); w.Code != http.StatusNoContent {
			t.Fatalf("put %s: %d %s", id, w.Code, w.Body)
		}
	}
	w := request(client, "GET", "/api/collector/catalog?verdict=not_for_me", "")
	var view struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || w.Code != http.StatusOK || len(view.Items) != 1 || view.Items[0]["gallery_id"] != float64(3) {
		t.Fatalf("not_for_me view: %d %s, %v", w.Code, w.Body, err)
	}
	if w := request(client, "GET", "/api/collector/catalog?verdict=maybe", ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_query") {
		t.Fatalf("unknown verdict view: %d %s", w.Code, w.Body)
	}
	before := requests.Load()
	for _, tc := range []struct{ method, path, body, code string }{
		{"PUT", path, `{"verdict":"maybe"}`, "invalid_verdict"},
		{"PUT", "/api/collector/catalog/verdicts/0", `{"verdict":"interesting"}`, "invalid_gallery_reference"},
		{"PUT", "/api/collector/catalog/verdicts/-1", `{"verdict":"interesting"}`, "invalid_gallery_reference"},
		{"PUT", "/api/collector/catalog/verdicts/x", `{"verdict":"interesting"}`, "invalid_gallery_reference"},
		{"DELETE", "/api/collector/catalog/verdicts/0", "", "invalid_gallery_reference"},
		{"DELETE", "/api/collector/catalog/verdicts/x", "", "invalid_gallery_reference"},
	} {
		if w := request(client, tc.method, tc.path, tc.body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%s %s %s: %d %s", tc.method, tc.path, tc.body, w.Code, w.Body)
		}
	}
	if requests.Load() != before {
		t.Fatal("invalid requests reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "PUT", path, `{"verdict":"interesting"}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "DELETE", path, ""); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalScoreOverrides(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'token')"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 2, Title: "Gallery 2", Posted: 2, Tags: []string{"female:big hat"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/score/overrides"
	if w := request(client, "POST", path, `{"kind":"tag","value":"female:big_hat","rule":"always_hide"}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("post: %d %s", w.Code, w.Body)
	}
	var id int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM panda_score_overrides WHERE kind = 'tag' AND rule = 'always_hide'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		body   string
		status int
		code   string
	}{
		{`{"kind":"tag","value":"female:glasses","rule":"always_hide"}`, http.StatusNotFound, "tag_not_found"},
		{`{"kind":"tag","value":"glasses","rule":"always_hide"}`, http.StatusBadRequest, "invalid_override"},
	} {
		if w := request(client, "POST", path, tc.body); w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("post %s: %d %s", tc.body, w.Code, w.Body)
		}
	}
	if w := request(client, "DELETE", path+"/"+strconv.FormatInt(id, 10), ""); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_overrides").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows after delete = %d, %v", count, err)
	}
	before := requests.Load()
	for _, invalid := range []string{"0", "-1", "x"} {
		if w := request(client, "DELETE", path+"/"+invalid, ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_override") {
			t.Fatalf("delete %s: %d %s", invalid, w.Code, w.Body)
		}
	}
	if requests.Load() != before {
		t.Fatal("invalid requests reached the collector")
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"kind":"uploader","value":"Some One","rule":"never_hide"}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "DELETE", path+"/1", ""); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalScorePins(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'token')"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 2, Title: "Gallery 2", Posted: 2, Tags: []string{"female:big hat"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: catalog.New(db, "https://panda.test"), APIToken: "server-secret"})
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/score/pins"
	w := request(client, "POST", path, `{"kind":"tag","value":"female:big_hat","weight":1.5}`)
	var pin map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &pin); err != nil || w.Code != http.StatusOK {
		t.Fatalf("post: %d %s, %v", w.Code, w.Body, err)
	}
	id, _ := pin["id"].(float64)
	if id <= 0 || pin["kind"] != "tag" || pin["label"] != "female:big hat" || pin["weight"] != 1.5 {
		t.Fatalf("pin = %v", pin)
	}
	for _, tc := range []struct {
		body   string
		status int
		code   string
	}{
		{`{"kind":"tag","value":"female:glasses","weight":1}`, http.StatusNotFound, "tag_not_found"},
		{`{"kind":"tag","value":"glasses","weight":1}`, http.StatusBadRequest, "invalid_pin"},
	} {
		if w := request(client, "POST", path, tc.body); w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("post %s: %d %s", tc.body, w.Code, w.Body)
		}
	}
	if w := request(client, "DELETE", path+"/"+strconv.FormatInt(int64(id), 10), ""); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_pins").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows after delete = %d, %v", count, err)
	}
	before := requests.Load()
	for _, invalid := range []string{"0", "-1", "x"} {
		if w := request(client, "DELETE", path+"/"+invalid, ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_pin") {
			t.Fatalf("delete %s: %d %s", invalid, w.Code, w.Body)
		}
	}
	for _, body := range []string{`{"kind":"uploader","value":"Some One"}`, `{"kind":"uploader","value":"Some One","weight":null}`} {
		if w := request(client, "POST", path, body); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_pin") {
			t.Fatalf("post %s: %d %s", body, w.Code, w.Body)
		}
	}
	if requests.Load() != before {
		t.Fatal("invalid requests reached the collector")
	}
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_pins").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rows after invalid posts = %d, %v", count, err)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "POST", path, `{"kind":"uploader","value":"Some One","weight":1}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token post: %d %s", w.Code, w.Body)
	}
	if w := request(wrong, "DELETE", path+"/1", ""); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token delete: %d %s", w.Code, w.Body)
	}
	for _, method := range []string{"POST", "DELETE"} {
		target, body := path, `{"kind":"uploader","value":"Some One","weight":1}`
		if method == "DELETE" {
			target, body = path+"/1", ""
		}
		if w := request(nil, method, target, body); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
			t.Fatalf("unconfigured %s: %d %s", method, w.Code, w.Body)
		}
	}
}

func TestLocalScoreTagWeights(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES (2, 'token')"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: 2, Title: "Gallery 2", Posted: 2, Tags: []string{"female:big hat", "male:hat"}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	pin, err := service.SetScorePin(t.Context(), collectorapi.ScoreKindTag, "female:big hat", 1.5)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/collector/catalog/score/tags", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const body = `{"tags":["female:big_hat","male:hat","other:none"]}`
	w := request(client, body)
	var weights struct {
		Active bool                         `json:"active"`
		Tags   []map[string]json.RawMessage `json:"tags"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &weights); err != nil || w.Code != http.StatusOK || weights.Active || len(weights.Tags) != 3 {
		t.Fatalf("post: %d %s, %v", w.Code, w.Body, err)
	}
	var gotPin collectorapi.ScorePin
	if err := json.Unmarshal(weights.Tags[0]["pin"], &gotPin); err != nil || gotPin != pin || string(weights.Tags[0]["weight"]) != "1.5" {
		t.Fatalf("pinned tag = %s, %v", w.Body, err)
	}
	for _, entry := range weights.Tags[1:] {
		if _, ok := entry["pin"]; ok {
			t.Fatalf("unpinned tag = %s", w.Body)
		}
	}
	if string(weights.Tags[1]["known"]) != "true" || string(weights.Tags[2]["known"]) != "false" {
		t.Fatalf("known = %s", w.Body)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, body); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token: %d %s", w.Code, w.Body)
	}
	if w := request(nil, body); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured: %d %s", w.Code, w.Body)
	}
}

func TestLocalCatalogScoreFilter(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	for id, tags := range map[int64][]string{2: {"female:big hat"}, 3: nil} {
		if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: "Gallery", Posted: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	request := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		return w
	}
	page := func(path string) map[string]any {
		t.Helper()
		w := request(path)
		var page map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s, %v", path, w.Code, w.Body, err)
		}
		return page
	}
	unfiltered := page("/api/collector/catalog")
	if _, ok := unfiltered["score_filter"]; ok {
		t.Fatalf("page without a filter in force: %v", unfiltered)
	}
	for _, item := range unfiltered["items"].([]any) {
		if _, ok := item.(map[string]any)["score"]; ok {
			t.Fatalf("item without a filter in force: %v", item)
		}
	}
	if err := service.SetScoreOverride(t.Context(), collectorapi.ScoreKindTag, "female:big hat", collectorapi.ScoreRuleAlwaysHide); err != nil {
		t.Fatal(err)
	}
	hidden := page("/api/collector/catalog?score_filter=hidden")
	items := hidden["items"].([]any)
	if _, ok := hidden["score_filter"].(map[string]any); !ok || len(items) != 1 {
		t.Fatalf("hidden page: %v", hidden)
	}
	item := items[0].(map[string]any)
	if score, _ := item["score"].(map[string]any); item["gallery_id"] != float64(2) || score["rule"] != "always_hide" || score["override"] != "female:big hat" {
		t.Fatalf("hidden item: %v", item)
	}
	if tags, _ := item["summary_tags"].([]any); len(tags) == 0 || tags[0].(map[string]any)["override"] != "always_hide" {
		t.Fatalf("hidden item tags: %v", item["summary_tags"])
	}
	if w := request("/api/collector/catalog?score_filter=bogus"); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_query") {
		t.Fatalf("bogus filter: %d %s", w.Code, w.Body)
	}
}

func TestLocalScoreModel(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	for id, tags := range map[int64][]string{2: {"male:hat"}, 3: {"female:glasses"}} {
		if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: "Gallery", Posted: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for id, verdict := range map[int64]string{2: collectorapi.VerdictInteresting, 3: collectorapi.VerdictNotForMe} {
		if err := service.SetVerdict(t.Context(), id, verdict); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	collectorHandler := collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"})
	var previewBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/catalog/score/preview" {
			previewBody, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(previewBody))
		}
		collectorHandler.ServeHTTP(w, r)
	}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	defaults := catalogfilter.New(pgtest.Open(t, localstorage.Migrations()))
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client, CatalogFilter: defaults}).ServeHTTP(w, r)
		return w
	}
	sent := func() map[string]json.RawMessage {
		t.Helper()
		var body map[string]json.RawMessage
		if err := json.Unmarshal(previewBody, &body); err != nil {
			t.Fatalf("preview request body %s: %v", previewBody, err)
		}
		return body
	}
	const previewPath = "/api/collector/catalog/score/preview"
	preview := func(body string) collectorapi.ScorePreview {
		t.Helper()
		w := request(client, "POST", previewPath, body)
		var result collectorapi.ScorePreview
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusOK {
			t.Fatalf("preview %s: %d %s, %v", body, w.Code, w.Body, err)
		}
		return result
	}
	scoreFilter := func() map[string]any {
		t.Helper()
		w := request(client, "GET", "/api/collector/catalog", "")
		var page map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
			t.Fatalf("catalog: %d %s, %v", w.Code, w.Body, err)
		}
		filter, _ := page["score_filter"].(map[string]any)
		return filter
	}

	result := preview(`{"favorites":false}`)
	if result.Evidence.Interesting != 1 || result.Evidence.NotForMe != 1 || result.Candidate == nil || result.Candidate.ID == 0 {
		t.Fatalf("preview: %+v", result)
	}
	if body := sent(); string(body["favorites"]) != "false" || string(body["default_query"]) != `""` || string(body["default_categories"]) != "[]" {
		t.Fatalf("empty default filter sent as %s", previewBody)
	}
	for _, body := range []string{`{}`, `{"favorites":null}`} {
		preview(body)
		if favorites := string(sent()["favorites"]); favorites != "null" {
			t.Fatalf("%s sent favorites %s", body, favorites)
		}
	}
	filter, err := catalogfilter.Normalize(catalogfilter.Filter{Query: "hat", Categories: []string{"Manga", "Doujinshi"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := defaults.Save(t.Context(), filter); err != nil {
		t.Fatal(err)
	}
	result = preview(`{"favorites":true}`)
	if body := sent(); string(body["default_query"]) != `"hat"` || string(body["default_categories"]) != `["manga","doujinshi"]` {
		t.Fatalf("default filter sent as %s", previewBody)
	}
	if err := defaults.Save(t.Context(), catalogfilter.Filter{}); err != nil {
		t.Fatal(err)
	}
	pin, err := service.SetScorePin(t.Context(), collectorapi.ScoreKindTag, "male:hat", 2.5)
	if err != nil {
		t.Fatal(err)
	}
	w := request(client, "POST", previewPath, `{"favorites":false}`)
	var pinned struct {
		Pins []map[string]any `json:"pins"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &pinned); err != nil || w.Code != http.StatusOK {
		t.Fatalf("preview with a pin: %d %s, %v", w.Code, w.Body, err)
	}
	if want := []map[string]any{{"id": float64(pin.ID), "kind": "tag", "label": "male:hat", "weight": 2.5}}; !reflect.DeepEqual(pinned.Pins, want) {
		t.Fatalf("preview pins = %v, want %v", pinned.Pins, want)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_authors (tag_id, favorites)
		SELECT id, 4 FROM catalog_tags WHERE namespace = 'female' AND value = 'glasses'`); err != nil {
		t.Fatal(err)
	}
	bonus := 3.0
	if err := service.SetAuthorBonus(t.Context(), &bonus); err != nil {
		t.Fatal(err)
	}
	w = request(client, "POST", previewPath, `{"favorites":false}`)
	var authors map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &authors); err != nil || w.Code != http.StatusOK {
		t.Fatalf("preview with a favorite author: %d %s, %v", w.Code, w.Body, err)
	}
	if got, want := []any{authors["author_bonus"], authors["favorite_author_count"], authors["favorite_authors"]},
		[]any{3.0, 1.0, []any{map[string]any{"label": "female:glasses", "favorites": 4.0, "weight": 3.0}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("preview bonus, count and favorite authors = %v, want %v", got, want)
	}
	if err := service.SetAuthorBonus(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM favorite_authors"); err != nil {
		t.Fatal(err)
	}
	result = preview(`{"favorites":false}`)

	const applyPath = "/api/collector/catalog/score/apply"
	const thresholdPath = "/api/collector/catalog/score/threshold"
	const modelPath = "/api/collector/catalog/score/model"
	candidate := strconv.FormatInt(result.Candidate.ID, 10)
	for _, tc := range []struct {
		method, path, body string
		status             int
		code               string
	}{
		{"POST", applyPath, `{"candidate_id":` + candidate + `9,"threshold_tenths":5}`, http.StatusConflict, "score_candidate_not_found"},
		{"POST", applyPath, `{"candidate_id":` + candidate + `,"threshold_tenths":2147483648}`, http.StatusBadRequest, "invalid_threshold"},
		{"PUT", thresholdPath, `{"threshold_tenths":5}`, http.StatusConflict, "score_model_not_active"},
	} {
		if w := request(client, tc.method, tc.path, tc.body); w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Fatalf("%s %s %s: %d %s", tc.method, tc.path, tc.body, w.Code, w.Body)
		}
	}
	if w := request(client, "POST", applyPath, `{"candidate_id":`+candidate+`,"threshold_tenths":-5}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("apply: %d %s", w.Code, w.Body)
	}
	if filter := scoreFilter(); filter["threshold_tenths"] != float64(-5) {
		t.Fatalf("score filter after apply: %v", filter)
	}
	if w := request(client, "PUT", thresholdPath, `{"threshold_tenths":7}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("threshold: %d %s", w.Code, w.Body)
	}
	if filter := scoreFilter(); filter["threshold_tenths"] != float64(7) {
		t.Fatalf("score filter after threshold: %v", filter)
	}
	if w := request(client, "DELETE", modelPath, ""); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("remove: %d %s", w.Code, w.Body)
	}
	if filter := scoreFilter(); filter != nil {
		t.Fatalf("score filter after remove: %v", filter)
	}

	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	unreachable, err := collectorapi.NewClient(closed.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct{ method, path, body, unknown string }{
		{"POST", previewPath, `{"favorites":true}`, `{"favorites":true,"default_query":"x"}`},
		{"POST", applyPath, `{"candidate_id":1,"threshold_tenths":0}`, `{"candidate_id":1,"threshold":0}`},
		{"PUT", thresholdPath, `{"threshold_tenths":0}`, `{"threshold":0}`},
		{"DELETE", modelPath, "", ""},
	} {
		for _, tc := range []struct {
			client *collectorapi.Client
			status int
			code   string
		}{
			{nil, http.StatusServiceUnavailable, "collector_not_configured"},
			{wrong, http.StatusBadGateway, "collector_unauthorized"},
			{unreachable, http.StatusBadGateway, "collector_unavailable"},
		} {
			if w := request(tc.client, route.method, route.path, route.body); w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
				t.Fatalf("%s %s with %s: %d %s", route.method, route.path, tc.code, w.Code, w.Body)
			}
		}
		if route.unknown != "" {
			if w := request(client, route.method, route.path, route.unknown); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_json") {
				t.Fatalf("%s %s %s: %d %s", route.method, route.path, route.unknown, w.Code, w.Body)
			}
		}
	}
}

func TestLocalCatalogSummaryTagsRankWeightedTags(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	galleries := map[int64][]string{7: {"artist:zed", "male:hat"}, 8: nil}
	for id := int64(1); id <= 6; id++ {
		galleries[id] = []string{"female:glasses"}
		if id > 3 {
			galleries[id] = []string{"male:hat"}
		}
	}
	for id, tags := range galleries {
		if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: "Gallery", Posted: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1); id <= 6; id++ {
		verdict := collectorapi.VerdictNotForMe
		if id > 3 {
			verdict = collectorapi.VerdictInteresting
		}
		if err := service.SetVerdict(t.Context(), id, verdict); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	summaryTags := func(path string) map[float64][]any {
		t.Helper()
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var page struct {
			Items []map[string]any `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s, %v", path, w.Code, w.Body, err)
		}
		tags := map[float64][]any{}
		for _, item := range page.Items {
			list, ok := item["summary_tags"].([]any)
			if !ok {
				t.Fatalf("%s: summary tags of %v", path, item)
			}
			tags[item["gallery_id"].(float64)] = list
		}
		return tags
	}

	unfiltered := summaryTags("/api/collector/catalog")
	for id, tags := range unfiltered {
		for _, tag := range tags {
			entry := tag.(map[string]any)
			if _, ok := entry["term"].(string); !ok || len(entry) != 3 || entry["namespace"] == nil || entry["value"] == nil {
				t.Fatalf("gallery %v summary tag without a filter in force: %v", id, entry)
			}
		}
	}
	if tags := unfiltered[8]; len(tags) != 0 {
		t.Fatalf("gallery without tags: %v", tags)
	}

	preview, err := service.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{})
	if err != nil || preview.Candidate == nil {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if err := service.ApplyScoreModel(t.Context(), preview.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	scored := summaryTags("/api/collector/catalog?score_filter=all")
	if tags := scored[8]; tags == nil || len(tags) != 0 {
		t.Fatalf("gallery without tags under a model: %v", tags)
	}
	tags := scored[7]
	if len(tags) != 2 {
		t.Fatalf("gallery 7 summary tags: %v", tags)
	}
	first, second := tags[0].(map[string]any), tags[1].(map[string]any)
	if _, ok := first["weight"].(float64); !ok || first["value"] != "hat" {
		t.Fatalf("first summary tag: %v", first)
	}
	if _, ok := second["weight"]; ok || second["value"] != "zed" {
		t.Fatalf("second summary tag: %v", second)
	}
}

func TestLocalCatalogMarksPinnedWeights(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	galleries := map[int64][]string{7: {"artist:zed", "male:hat"}}
	for id := int64(1); id <= 6; id++ {
		galleries[id] = []string{"female:glasses"}
		if id > 3 {
			galleries[id] = []string{"male:hat"}
		}
	}
	for id, tags := range galleries {
		if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: "Gallery", Posted: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1); id <= 6; id++ {
		verdict := collectorapi.VerdictNotForMe
		if id > 3 {
			verdict = collectorapi.VerdictInteresting
		}
		if err := service.SetVerdict(t.Context(), id, verdict); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := service.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{})
	if err != nil || preview.Candidate == nil {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if err := service.ApplyScoreModel(t.Context(), preview.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetScorePin(t.Context(), collectorapi.ScoreKindTag, "artist:zed", 3); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"}))
	defer upstream.Close()
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, httptest.NewRequest("GET", "/api/collector/catalog?score_filter=all", nil))
	var page struct {
		Items []struct {
			GalleryID   int64            `json:"gallery_id"`
			SummaryTags []map[string]any `json:"summary_tags"`
			Score       struct {
				Weights []map[string]any `json:"weights"`
			} `json:"score"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s, %v", w.Code, w.Body, err)
	}
	for _, item := range page.Items {
		if item.GalleryID != 7 {
			continue
		}
		if len(item.SummaryTags) != 2 || len(item.Score.Weights) != 2 {
			t.Fatalf("gallery 7: %+v", item)
		}
		for _, entries := range [][]map[string]any{item.SummaryTags, item.Score.Weights} {
			pinned, other := entries[0], entries[1]
			if pinned["source"] != "pin" || pinned["weight"] != 3.0 {
				t.Fatalf("pinned entry: %v", pinned)
			}
			if _, ok := other["source"]; ok {
				t.Fatalf("calculated entry: %v", other)
			}
		}
		return
	}
	t.Fatalf("gallery 7 not listed: %s", w.Body)
}

func TestLocalAuthorBonus(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	service := catalog.New(db, "https://panda.test")
	galleries := map[int64][]string{7: {"artist:zed", "male:hat"}}
	for id := int64(1); id <= 6; id++ {
		galleries[id] = []string{"female:glasses"}
		if id > 3 {
			galleries[id] = []string{"male:hat"}
		}
	}
	for id, tags := range galleries {
		if _, err := db.Exec(t.Context(), "INSERT INTO gallery_refs (gallery_id, token) VALUES ($1, 'token')", id); err != nil {
			t.Fatal(err)
		}
		tx, err := db.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := catalog.Project(t.Context(), tx, panda.Metadata{ID: id, Title: "Gallery", Posted: id, Tags: tags}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	for id := int64(1); id <= 6; id++ {
		verdict := collectorapi.VerdictNotForMe
		if id > 3 {
			verdict = collectorapi.VerdictInteresting
		}
		if err := service.SetVerdict(t.Context(), id, verdict); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := service.PreviewScoreModel(t.Context(), collectorapi.ScorePreviewOptions{})
	if err != nil || preview.Candidate == nil {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	if err := service.ApplyScoreModel(t.Context(), preview.Candidate.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(t.Context(), `INSERT INTO favorite_authors (tag_id, favorites)
		SELECT id, 2 FROM catalog_tags WHERE namespace = 'artist' AND value = 'zed'`); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	upstream := httptest.NewServer(collectorhttp.NewHandler(&collector.App{Logger: logger, Catalog: service, APIToken: "server-secret"}))
	defer upstream.Close()
	request := func(client *collectorapi.Client, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		NewHandler(&local.App{Logger: logger, Collector: client}).ServeHTTP(w, r)
		return w
	}
	client, err := collectorapi.NewClient(upstream.URL, "server-secret")
	if err != nil {
		t.Fatal(err)
	}
	const path = "/api/collector/catalog/score/author-bonus"
	if w := request(client, "PUT", path, `{"bonus":2.5}`); w.Code != http.StatusNoContent || w.Body.Len() != 0 {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	var bonus *float64
	if err := db.QueryRow(t.Context(), "SELECT author_bonus FROM panda_score_settings").Scan(&bonus); err != nil || bonus == nil || *bonus != 2.5 {
		t.Fatalf("stored bonus = %v, %v", bonus, err)
	}
	if w := request(client, "PUT", path, `{"bonus":0}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid_author_bonus") {
		t.Fatalf("invalid put: %d %s", w.Code, w.Body)
	}
	wrong, err := collectorapi.NewClient(upstream.URL, "wrong-secret")
	if err != nil {
		t.Fatal(err)
	}
	if w := request(wrong, "PUT", path, `{"bonus":1}`); w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "collector_unauthorized") {
		t.Fatalf("wrong token put: %d %s", w.Code, w.Body)
	}
	if w := request(nil, "PUT", path, `{"bonus":1}`); w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "collector_not_configured") {
		t.Fatalf("unconfigured put: %d %s", w.Code, w.Body)
	}

	w := request(client, "GET", "/api/collector/catalog?score_filter=all", "")
	var page struct {
		Items []struct {
			GalleryID   int64            `json:"gallery_id"`
			SummaryTags []map[string]any `json:"summary_tags"`
		} `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil || w.Code != http.StatusOK {
		t.Fatalf("%d %s, %v", w.Code, w.Body, err)
	}
	for _, item := range page.Items {
		if item.GalleryID != 7 {
			continue
		}
		for _, tag := range item.SummaryTags {
			favorite, ok := tag["favorite_author"]
			if tag["value"] == "zed" {
				if favorite != true || tag["source"] != "author_bonus" || tag["weight"] != 2.5 {
					t.Fatalf("favorite author tag: %v", tag)
				}
			} else if ok {
				t.Fatalf("other tag: %v", tag)
			}
		}
		return
	}
	t.Fatalf("gallery 7 not listed: %s", w.Body)
}
