package metadataproxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testService(t *testing.T, dir, apiURL string) *Service {
	t.Helper()
	return testServiceWithIPCheck(t, dir, apiURL, func(context.Context, http.RoundTripper) error { return nil })
}

func testServiceWithIPCheck(t *testing.T, dir, apiURL string, verify func(context.Context, http.RoundTripper) error) *Service {
	t.Helper()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// Main collection remains paused throughout these tests. Proxies must make
	// progress without resetting or bypassing its persisted retry state.
	if _, err := db.Exec(`UPDATE metadata_retry SET next_attempt_at = ?, failures = 3`, time.Now().Add(time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.DiscardHandler)
	mainClient, err := panda.NewClient(apiURL, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("main must remain paused")
	})})
	if err != nil {
		t.Fatal(err)
	}
	main := metadata.New(t.Context(), db, mainClient, logger)
	s, err := newService(t.Context(), db, main, panda.Config{APIURL: apiURL, RateInterval: 10 * time.Millisecond}, logger, verify)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); main.Close(); db.Close() })
	return s
}

func seed(t *testing.T, db *sql.DB, n int) {
	t.Helper()
	for id := 1; id <= n; id++ {
		if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?)`, id, fmt.Sprintf("token%d", id)); err != nil {
			t.Fatal(err)
		}
	}
}

func addChannel(t *testing.T, s *Service, name, address string) string {
	t.Helper()
	if err := s.Save(t.Context(), "", collectorapi.MetadataProxyInput{Name: name, ProxyURL: address, UserAgent: "Test Browser", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, ch := range status(t, s).Channels {
		if ch.Name == name {
			return ch.ID
		}
	}
	t.Fatal("channel missing after save")
	return ""
}

func TestImportPersistsObservedBan(t *testing.T) {
	s := testService(t, t.TempDir(), "http://panda.invalid/api")
	until := time.Now().Add(time.Hour).UnixMilli()
	s.mu.Lock()
	s.observed["endpoint:socks5://proxy.invalid:1080"] = until
	s.mu.Unlock()
	result, err := s.Import(t.Context(), collectorapi.MetadataProxyImportInput{Proxies: "proxy.invalid:1080", Protocol: "socks5"})
	if err != nil || result.Added != 1 {
		t.Fatalf("import: %+v, %v", result, err)
	}
	// Endpoint history must survive removal of the imported channel and restart.
	var recorded int64
	if err := s.db.QueryRow(`SELECT until_at FROM metadata_proxy_bans WHERE endpoint = ?`, "socks5://proxy.invalid:1080").Scan(&recorded); err != nil || recorded != until {
		t.Fatalf("ban history = %d, %v", recorded, err)
	}
}

func status(t *testing.T, s *Service) collectorapi.MetadataProxyStatus {
	t.Helper()
	result, err := s.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func awaitStatus(t *testing.T, s *Service, match func(collectorapi.MetadataProxyStatus) bool) collectorapi.MetadataProxyStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		result := status(t, s)
		if match(result) {
			return result
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy status did not settle: %+v", result)
		}
		time.Sleep(time.Millisecond)
	}
}

func receive[T any](t *testing.T, c <-chan T) T {
	t.Helper()
	select {
	case value := <-c:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("proxy request did not arrive")
	}
	var zero T
	return zero
}

func respond(t *testing.T, w http.ResponseWriter, r *http.Request) []panda.Metadata {
	t.Helper()
	var request struct {
		GIDList [][2]json.RawMessage `json:"gidlist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Error(err)
		return nil
	}
	var entries []panda.Metadata
	for _, pair := range request.GIDList {
		var entry panda.Metadata
		if err := json.Unmarshal(pair[0], &entry.ID); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal(pair[1], &entry.Token); err != nil {
			t.Error(err)
		}
		entries = append(entries, entry)
	}
	if w != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"gmetadata": entries})
	}
	return entries
}

func TestChannelsOverlapAndDisableFinishesAssignedBatches(t *testing.T) {
	started := make(chan []panda.Metadata, 4)
	release := make(chan struct{})
	proxy := func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.IsAbs() || r.URL.Host != "panda.invalid" || r.Header.Get("User-Agent") != "Test Browser" || r.Header.Get("Cookie") != "" {
			t.Error("request did not use configured proxy route and identity")
		}
		entries := respond(t, nil, r)
		started <- entries
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"gmetadata": entries})
	}
	a, b := httptest.NewServer(http.HandlerFunc(proxy)), httptest.NewServer(http.HandlerFunc(proxy))
	defer a.Close()
	defer b.Close()
	s := testService(t, t.TempDir(), "http://panda.invalid/api")
	defer s.Close()
	// A shared limiter could not start both first batches within the deadline.
	s.mu.Lock()
	s.config.RateInterval = time.Hour
	s.mu.Unlock()
	seed(t, s.db, 51)
	addChannel(t, s, "A", a.URL)
	addChannel(t, s, "B", b.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	first, second := receive(t, started), receive(t, started)
	if len(first) != 25 || len(second) != 25 {
		t.Fatalf("batch sizes = %d, %d", len(first), len(second))
	}
	seen := map[int64]bool{}
	for _, entries := range [][]panda.Metadata{first, second} {
		for _, entry := range entries {
			if seen[entry.ID] {
				t.Fatalf("duplicate in-flight gallery %d", entry.ID)
			}
			seen[entry.ID] = true
		}
	}
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	for _, ch := range status(t, s).Channels {
		if ch.State != "stopping" || ch.BatchSize != 25 {
			t.Fatalf("disable did not drain: %+v", ch)
		}
	}
	close(release)
	awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		for _, ch := range result.Channels {
			if ch.BatchSize != 0 {
				return false
			}
		}
		return true
	})
	var available, pending, failures int
	if err := s.db.QueryRow(`SELECT (SELECT count(*) FROM gallery_metadata), (SELECT count(*) FROM gallery_refs WHERE metadata_attempted_at IS NULL), (SELECT failures FROM metadata_retry)`).Scan(&available, &pending, &failures); err != nil {
		t.Fatal(err)
	}
	if available != 50 || pending != 1 || failures != 3 {
		t.Fatalf("available=%d pending=%d main failures=%d", available, pending, failures)
	}
	select {
	case <-started:
		t.Fatal("new batch started after disable")
	default:
	}
}

func TestBansAreIndependentAndSurviveEditsRemovalAndRestart(t *testing.T) {
	blocked := make(chan struct{})
	release := make(chan struct{})
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "Your IP is temporarily banned for excessive pageloads. Ban expires in 1 hour")
	}))
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case blocked <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		respond(t, w, r)
	}))
	defer a.Close()
	defer b.Close()
	dir := t.TempDir()
	s := testService(t, dir, "http://panda.invalid/api")
	defer s.Close()
	seed(t, s.db, 26)
	id := addChannel(t, s, "A", a.URL)
	addChannel(t, s, "B", b.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		return result.Channels[0].BanUntil != nil && result.Channels[1].BatchSize > 0
	})
	until := *result.Channels[0].BanUntil
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	close(release)
	awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool { return result.Channels[1].LastSuccessAt != nil })
	if status(t, s).Channels[0].BanUntil == nil {
		t.Fatal("another proxy's success cleared ban")
	}
	var mainBan int64
	if err := s.db.QueryRow(`SELECT until_at FROM panda_ban`).Scan(&mainBan); err != nil || mainBan != 0 {
		t.Fatalf("main ban = %d, %v", mainBan, err)
	}
	if err := s.Save(t.Context(), id, collectorapi.MetadataProxyInput{Name: "A edited", ProxyURL: a.URL, UserAgent: "Other Browser", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool { return len(result.Channels) == 1 })
	id = addChannel(t, s, "A recreated", a.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.metadata.Close()
	s.db.Close()
	s = testService(t, dir, "http://panda.invalid/api")
	defer s.Close()
	for _, ch := range status(t, s).Channels {
		if ch.ID == id {
			if ch.BanUntil == nil || !ch.BanUntil.Equal(until) || ch.State != "waiting_cooldown" {
				t.Fatalf("ban lost after recreation/restart: %+v", ch)
			}
			return
		}
	}
	t.Fatal("recreated channel missing")
}

func TestEditingActiveRouteCarriesLateBanToNewEndpoint(t *testing.T) {
	started, release := make(chan struct{}, 1), make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "IP temporarily banned for excessive pageloads. Ban expires in 1 hour")
	}))
	defer proxy.Close()
	s := testService(t, t.TempDir(), "http://panda.invalid/api")
	defer s.Close()
	seed(t, s.db, 1)
	id := addChannel(t, s, "A", proxy.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	receive(t, started)
	if err := s.Save(t.Context(), id, collectorapi.MetadataProxyInput{Name: "New route", ProxyURL: "http://replacement.invalid:8080", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if status(t, s).Channels[0].State != "updating" {
		t.Fatal("route did not wait for active batch")
	}
	close(release)
	result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		return result.Channels[0].State == "waiting_cooldown"
	})
	var old, next int64
	if err := s.db.QueryRow(`SELECT (SELECT until_at FROM metadata_proxy_bans WHERE endpoint = ?), (SELECT until_at FROM metadata_proxy_bans WHERE endpoint = 'http://replacement.invalid:8080')`, proxy.URL).Scan(&old, &next); err != nil || old != next || next != result.Channels[0].BanUntil.UnixMilli() {
		t.Fatalf("late ban did not follow edit: %d/%d, %v", old, next, err)
	}
}

func TestProxyAuthenticationPausesHTTPAndCONNECTWithoutLosingWork(t *testing.T) {
	for _, scheme := range []string{"http", "https"} {
		t.Run(scheme, func(t *testing.T) {
			var calls atomic.Int64
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if scheme == "https" && r.Method != "CONNECT" {
					t.Error("expected CONNECT")
				}
				w.WriteHeader(http.StatusProxyAuthRequired)
				_, _ = io.WriteString(w, "private-password")
			}))
			defer proxy.Close()
			s := testService(t, t.TempDir(), scheme+"://panda.invalid/api")
			defer s.Close()
			seed(t, s.db, 1)
			id := addChannel(t, s, "A", proxy.URL)
			if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
				t.Fatal(err)
			}
			result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
				return result.Channels[0].State == "authentication_required"
			})
			body, _ := json.Marshal(result)
			if strings.Contains(string(body), "private-password") {
				t.Fatal("error exposed password")
			}
			var pending int
			if err := s.db.QueryRow(`SELECT count(*) FROM gallery_refs WHERE metadata_attempted_at IS NULL`).Scan(&pending); err != nil || pending != 1 {
				t.Fatalf("proxy error finished gallery: %d, %v", pending, err)
			}
			if err := s.Save(t.Context(), id, collectorapi.MetadataProxyInput{Name: "Renamed", ProxyURL: proxy.URL, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.dispatch(t.Context()); err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Fatal("name edit retried invalid credentials")
			}
			password := "corrected-password"
			if err := s.Save(t.Context(), id, collectorapi.MetadataProxyInput{Name: "A", ProxyURL: proxy.URL, Username: "user", Password: &password, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
				return calls.Load() == 2 && result.Channels[0].State == "authentication_required"
			})
		})
	}
}

func TestSOCKSAuthenticationFailurePausesChannel(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	finished := make(chan error, 1)
	go func() {
		finished <- func() error {
			conn, err := listener.Accept()
			if err != nil {
				return err
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return err
			}
			var prefix [2]byte
			if _, err := io.ReadFull(conn, prefix[:]); err != nil {
				return err
			}
			if _, err := io.CopyN(io.Discard, conn, int64(prefix[1])); err != nil {
				return err
			}
			if _, err := conn.Write([]byte{5, 2}); err != nil {
				return err
			}
			if _, err := io.ReadFull(conn, prefix[:]); err != nil {
				return err
			}
			if _, err := io.CopyN(io.Discard, conn, int64(prefix[1])); err != nil {
				return err
			}
			if _, err := io.ReadFull(conn, prefix[:1]); err != nil {
				return err
			}
			if _, err := io.CopyN(io.Discard, conn, int64(prefix[0])); err != nil {
				return err
			}
			_, err = conn.Write([]byte{1, 1}) // Reject username/password authentication.
			return err
		}()
	}()
	s := testService(t, t.TempDir(), "https://panda.invalid/api")
	defer s.Close()
	seed(t, s.db, 1)
	password := "test-password"
	if err := s.Save(t.Context(), "", collectorapi.MetadataProxyInput{Name: "SOCKS", ProxyURL: "socks5h://" + listener.Addr().String(), Username: "user", Password: &password, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		return result.Channels[0].State == "authentication_required"
	})
	if result.Channels[0].LastError != errProxyAuth.Error() {
		t.Fatalf("SOCKS error not classified: %+v", result)
	}
	if err := receive(t, finished); err != nil {
		t.Fatal(err)
	}
}

func TestProxyRequestRejectionRemainsPendingAndBackoffGrows(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer proxy.Close()
	s := testService(t, t.TempDir(), "http://panda.invalid/api")
	defer s.Close()
	seed(t, s.db, 1)
	id := addChannel(t, s, "A", proxy.URL)
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool { return result.Channels[0].State == "waiting_retry" })
	if _, err := s.db.Exec(`UPDATE metadata_proxy_channels SET retry_at = 0 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	s.notify()
	result := awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
		return result.Channels[0].RetryAt != nil && time.Until(*result.Channels[0].RetryAt) > 90*time.Second
	})
	if result.Channels[0].LastError != "upstream_http_403" {
		t.Fatalf("unexpected error: %+v", result)
	}
	batch, err := s.metadata.ClaimBackground(t.Context())
	if err != nil || batch == nil || batch.Size() != 1 {
		t.Fatalf("failed proxy lost work: %+v, %v", batch, err)
	}
	batch.Close()
}

func TestEndpointEditsDoNotReuseSavedPassword(t *testing.T) {
	replacement := "replacement-password"
	empty := ""
	for _, tt := range []struct {
		name, endpoint string
		password       *string
		want           string
	}{
		{name: "different host", endpoint: "http://other.example"},
		{name: "different port", endpoint: "http://proxy.example:8080"},
		{name: "different protocol", endpoint: "https://proxy.example:80"},
		{name: "replacement", endpoint: "http://other.example", password: &replacement, want: replacement},
		{name: "explicit clear", endpoint: "http://proxy.example", password: &empty},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := testService(t, t.TempDir(), "http://panda.invalid/api")
			password := "private-password"
			input := collectorapi.MetadataProxyInput{Name: "A", ProxyURL: "http://proxy.example", Username: "one", Password: &password}
			if err := s.Save(t.Context(), "", input); err != nil {
				t.Fatal(err)
			}
			id := status(t, s).Channels[0].ID
			input.ProxyURL, input.Password = tt.endpoint, tt.password
			if err := s.Save(t.Context(), id, input); err != nil {
				t.Fatal(err)
			}
			var saved string
			if err := s.db.QueryRow(`SELECT password FROM metadata_proxy_channels WHERE id = ?`, id).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			if saved != tt.want {
				t.Fatal("saved password does not match endpoint edit policy")
			}
			if status(t, s).Channels[0].HasPassword != (tt.want != "") {
				t.Fatal("password status does not match saved credentials")
			}
		})
	}
}

func TestConfigurationNormalizesIdentityAndPreservesPassword(t *testing.T) {
	s := testService(t, t.TempDir(), "http://panda.invalid/api")
	password := "private-password"
	input := collectorapi.MetadataProxyInput{Name: "A", ProxyURL: "http://PROXY.example.:080/", Username: "one", Password: &password}
	if err := s.Save(t.Context(), "", input); err != nil {
		t.Fatal(err)
	}
	ch := status(t, s).Channels[0]
	if ch.ProxyURL != "http://proxy.example:80" || !ch.HasPassword || ch.UserAgent != defaultUserAgent {
		t.Fatalf("configuration: %+v", ch)
	}
	input.ProxyURL, input.Username = "http://proxy.example", "two"
	if err := s.Save(t.Context(), "", input); !errors.Is(err, collectorapi.ErrDuplicateProxy) {
		t.Fatalf("duplicate: %v", err)
	}
	input.Password = nil
	if err := s.Save(t.Context(), ch.ID, input); err != nil {
		t.Fatal(err)
	}
	var saved string
	if err := s.db.QueryRow(`SELECT password FROM metadata_proxy_channels WHERE id = ?`, ch.ID).Scan(&saved); err != nil || saved != password {
		t.Fatal("edit did not retain password")
	}
	body, _ := json.Marshal(status(t, s))
	if strings.Contains(string(body), password) || strings.Contains(string(body), `"password":`) {
		t.Fatal("status exposed password")
	}
	for _, invalid := range []string{"", "ftp://proxy.example", "http://proxy.example:0", "http://proxy.example/path", "http://proxy.example?query"} {
		input.ProxyURL = invalid
		if err := s.Save(t.Context(), ch.ID, input); !errors.Is(err, collectorapi.ErrInvalidProxy) {
			t.Fatalf("accepted invalid proxy %q: %v", invalid, err)
		}
	}
	input.ProxyURL, input.UserAgent = "http://proxy.example", "Browser\r\nInjected: value"
	if err := s.Save(t.Context(), ch.ID, input); !errors.Is(err, collectorapi.ErrInvalidProxy) {
		t.Fatalf("accepted invalid header: %v", err)
	}
}
