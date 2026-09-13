package metadataproxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestStatusDuringLargeProxyDispatch(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO reference_imports (id, filename, status, created_at, size_bytes)
		VALUES ('large', 'large.txt', 'validating', 1, 0);
		WITH RECURSIVE ids(id) AS (VALUES(1) UNION ALL SELECT id + 1 FROM ids WHERE id < 20000)
		INSERT INTO reference_import_entries (import_id, gallery_id, token, status)
		SELECT 'large', id, 'token', 'pending' FROM ids;
		WITH RECURSIVE ids(id) AS (VALUES(1) UNION ALL SELECT id + 1 FROM ids WHERE id < 661)
		INSERT INTO metadata_proxy_channels(id,name,endpoint,username,password,user_agent,enabled,created_at)
		SELECT 'channel'||id, '', 'http://proxy'||id||'.invalid:80', '', '', 'test', 1, 1 FROM ids;
		UPDATE metadata_proxy_settings SET enabled=1, auto_remove_inactive=0;`)
	if err != nil {
		t.Fatal(err)
	}
	paused, stop := context.WithCancel(t.Context())
	stop()
	logger := slog.New(slog.DiscardHandler)
	main := metadata.New(paused, db, nil, logger)
	defer main.Close()
	ctx, cancel := context.WithCancel(t.Context())
	s := &Service{db: db, metadata: main, logger: logger, cancel: cancel, wake: make(chan struct{}, 1),
		config:  panda.Config{APIURL: "https://panda.invalid/api", RateInterval: time.Millisecond},
		runtime: make(map[string]*channelRuntime), observed: make(map[string]int64),
		verifyIP: func(ctx context.Context, _ http.RoundTripper) error { <-ctx.Done(); return ctx.Err() },
	}
	defer s.Close()
	dispatched := make(chan error, 1)
	start := time.Now()
	go func() { dispatched <- s.dispatch(ctx) }()
	// Start the status request while dispatch owns the configuration lock.
	// Waiting on all channels would exceed the API deadline at this scale.
	for s.mu.TryLock() {
		s.mu.Unlock()
		select {
		case err := <-dispatched:
			t.Fatalf("dispatch finished before status probe: %v", err)
		default:
			runtime.Gosched()
		}
	}
	statusCtx, statusCancel := context.WithTimeout(ctx, 4*time.Second)
	defer statusCancel()
	result, statusErr := s.Status(statusCtx)
	t.Logf("status elapsed: %s, error: %v", time.Since(start), statusErr)
	cancel()
	if err := <-dispatched; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if statusErr != nil {
		t.Fatal(statusErr)
	}
	if len(result.Channels) != 661 {
		t.Fatalf("proxy status omitted channels: %d", len(result.Channels))
	}
}
