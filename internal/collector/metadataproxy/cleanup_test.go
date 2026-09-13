package metadataproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestCleanupRemovesChannelsWithoutRecentSuccess(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		cleanup, enabled, auth bool
		createdAge, successAge time.Duration
		wantRemoved            bool
	}{
		{name: "off by default", createdAge: time.Hour, successAge: 6 * time.Minute},
		{name: "recent success", cleanup: true, createdAge: time.Hour, successAge: 4 * time.Minute},
		{name: "expired success", cleanup: true, enabled: true, createdAge: time.Hour, successAge: 6 * time.Minute, wantRemoved: true},
		{name: "new channel", cleanup: true, createdAge: time.Minute},
		{name: "never succeeded", cleanup: true, createdAge: 6 * time.Minute, wantRemoved: true},
		{name: "disabled channel", cleanup: true, createdAge: time.Hour, successAge: 6 * time.Minute, wantRemoved: true},
		{name: "authentication failed", cleanup: true, enabled: true, auth: true, createdAge: 6 * time.Minute, wantRemoved: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t, t.TempDir(), "http://panda.invalid/api")
			s.Close() // Dispatch explicitly so timestamp setup cannot race cleanup.
			id := addChannel(t, s, "A", "http://proxy.invalid:8080")
			now := time.Now()
			var success int64
			if tc.successAge > 0 {
				success = now.Add(-tc.successAge).UnixMilli()
			}
			if _, err := s.db.Exec(`UPDATE metadata_proxy_channels SET created_at = ?, last_success_at = ?, enabled = ?, auth_failed = ?, retry_at = ? WHERE id = ?`,
				now.Add(-tc.createdAge).UnixMilli(), success, tc.enabled, tc.auth, now.Add(time.Hour).UnixMilli(), id); err != nil {
				t.Fatal(err)
			}
			if tc.cleanup {
				if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{AutoRemoveInactive: true}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.dispatch(t.Context()); err != nil {
				t.Fatal(err)
			}
			if result := status(t, s); (len(result.Channels) == 0) != tc.wantRemoved || result.AutoRemoveInactive != tc.cleanup {
				t.Fatalf("cleanup: %+v; want removed=%v", result, tc.wantRemoved)
			}
		})
	}
}

func TestCleanupPersistsAndGivesImportedChannelsInitialGrace(t *testing.T) {
	dir := t.TempDir()
	s := testService(t, dir, "http://panda.invalid/api")
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{AutoRemoveInactive: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(t.Context(), collectorapi.MetadataProxyImportInput{Proxies: "proxy.invalid:1080", Protocol: "socks5"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = testService(t, dir, "http://panda.invalid/api")
	if err := s.dispatch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if result := status(t, s); !result.AutoRemoveInactive || len(result.Channels) != 1 || result.Channels[0].LastSuccessAt != nil {
		t.Fatalf("restarted cleanup: %+v", result)
	}
	if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE metadata_proxy_channels SET created_at = ?`, time.Now().Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(t.Context()); err != nil {
		t.Fatal(err)
	}
	if result := status(t, s); result.AutoRemoveInactive || len(result.Channels) != 1 {
		t.Fatalf("disabled cleanup: %+v", result)
	}
}

func TestCleanupLetsActiveBatchFinish(t *testing.T) {
	for _, success := range []bool{true, false} {
		t.Run(map[bool]string{true: "success resets timer", false: "ban survives removal"}[success], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				entries := respond(t, nil, r)
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if success {
					_ = json.NewEncoder(w).Encode(map[string]any{"gmetadata": entries})
				} else {
					_, _ = io.WriteString(w, "Your IP is temporarily banned for excessive pageloads. Ban expires in 1 hour.")
				}
			}))
			defer proxy.Close()
			s := testService(t, t.TempDir(), "http://panda.invalid/api")
			defer s.Close()
			seed(t, s.db, 1)
			id := addChannel(t, s, "A", proxy.URL)
			if err := s.SetSettings(t.Context(), collectorapi.MetadataProxySettings{Enabled: true, AutoRemoveInactive: true}); err != nil {
				t.Fatal(err)
			}
			receive(t, started)
			if _, err := s.db.Exec(`UPDATE metadata_proxy_channels SET created_at = ?, last_success_at = ? WHERE id = ?`,
				time.Now().Add(-time.Hour).UnixMilli(), time.Now().Add(-6*time.Minute).UnixMilli(), id); err != nil {
				t.Fatal(err)
			}
			if err := s.dispatch(t.Context()); err != nil {
				t.Fatal(err)
			}
			if result := status(t, s); len(result.Channels) != 1 || result.Channels[0].State != "running" {
				t.Fatalf("active batch removed: %+v", result)
			}
			close(release)
			awaitStatus(t, s, func(result collectorapi.MetadataProxyStatus) bool {
				if !success {
					return len(result.Channels) == 0
				}
				return len(result.Channels) == 1 && result.Channels[0].LastSuccessAt != nil && time.Since(*result.Channels[0].LastSuccessAt) < time.Minute
			})
			if !success {
				addChannel(t, s, "Recreated", proxy.URL)
				if ch := status(t, s).Channels[0]; ch.BanUntil == nil || !ch.BanUntil.After(time.Now()) {
					t.Fatalf("removed channel lost ban: %+v", ch)
				}
			}
		})
	}
}
