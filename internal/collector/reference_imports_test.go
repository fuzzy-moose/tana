package collector_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector"
	collectorhttp "github.com/fuzzy-moose/tana/internal/collector/httpapi"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local"
	localhttp "github.com/fuzzy-moose/tana/internal/local/httpapi"
)

// Exercise acceptance through both HTTP hops, then resume with both applications
// recreated. The submitting Tana process is not an owner of accepted work.
func TestReferenceImportThroughTanaSurvivesRestart(t *testing.T) {
	requested := make(chan struct{}, 1)
	resume := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_, _ = io.WriteString(w, `<feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
			return
		}
		var input struct {
			Refs []json.RawMessage `json:"gidlist"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case requested <- struct{}{}:
		default:
		}
		select {
		case <-resume:
		case <-r.Context().Done():
			return
		}
		entries := make([]map[string]any, 0, len(input.Refs))
		for _, raw := range input.Refs {
			var pair []json.RawMessage
			if err := json.Unmarshal(raw, &pair); err != nil || len(pair) != 2 {
				http.Error(w, "invalid reference", http.StatusBadRequest)
				return
			}
			var id int64
			var token string
			_ = json.Unmarshal(pair[0], &id)
			_ = json.Unmarshal(pair[1], &token)
			entry := map[string]any{"gid": id, "token": token, "title": "Imported gallery"}
			if token == "bad" {
				entry = map[string]any{"gid": id, "error": "Invalid token"}
			}
			entries = append(entries, entry)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"gmetadata": entries})
	}))
	defer upstream.Close()

	values := map[string]string{
		"TANA_COLLECTOR_DATA_DIR": t.TempDir(), "TANA_COLLECTOR_API_TOKEN": "test-token",
		"PANDA_FEED_URL": upstream.URL, "PANDA_API_URL": upstream.URL, "PANDA_RATE_INTERVAL": "1ms",
		"PANDA_FAVORITES_URL": upstream.URL + "/favorites", "PANDA_ARCHIVER_URL": upstream.URL + "/archive",
		"PANDA_FAVORITES_COOKIES": `{"ipb_member_id":"test"}`,
	}
	cfg, err := collector.LoadConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	localDir := t.TempDir()
	start := func() (string, func()) {
		t.Helper()
		logger := slog.New(slog.DiscardHandler)
		app, err := collector.New(t.Context(), cfg, logger)
		if err != nil {
			t.Fatal(err)
		}
		remote := httptest.NewServer(collectorhttp.NewHandler(app))
		tana, err := local.New(t.Context(), local.Config{DataDir: localDir,
			CollectorURL: remote.URL, CollectorAPIToken: cfg.APIToken}, logger)
		if err != nil {
			remote.Close()
			_ = app.Close()
			t.Fatal(err)
		}
		browser := httptest.NewServer(localhttp.NewHandler(tana))
		return browser.URL, func() {
			browser.Close()
			_ = tana.Close()
			remote.Close()
			_ = app.Close()
		}
	}
	base, stop := start()
	stopped := false
	defer func() {
		if !stopped {
			stop()
		}
	}()
	request := func(method, path, body string, want int) collectorapi.ReferenceImport {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != want {
			data, _ := io.ReadAll(response.Body)
			t.Fatalf("%s %s: HTTP %d: %s", method, path, response.StatusCode, data)
		}
		var result collectorapi.ReferenceImport
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	const path = "/api/collector/reference-imports"
	input := "41,good\n41,good\ninvalid\n42,bad\n42,right\n"
	accepted := request(http.MethodPost, path+"?filename=references.txt", input, http.StatusAccepted)
	if accepted.ID == "" || accepted.Filename != "references.txt" || accepted.SizeBytes != int64(len(input)) {
		t.Fatalf("acceptance: %+v", accepted)
	}
	select {
	case <-requested:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted references were not scheduled")
	}
	stop()
	stopped = true
	close(resume)
	base, stop = start()
	stopped = false
	deadline := time.Now().Add(5 * time.Second)
	for {
		result := request(http.MethodGet, path+"/"+accepted.ID, "", http.StatusOK)
		if result.Status == "completed" {
			if result.References != 3 || result.Duplicates != 1 || result.Invalid != 1 ||
				result.Imported != 2 || result.Failed != 1 || result.Pending != 0 ||
				result.ProcessedBytes != int64(len(input)) || result.CompletedAt == nil {
				t.Fatalf("resumed summary: %+v", result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("import did not finish after restart: %+v", result)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
