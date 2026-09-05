package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/server"
)

func TestHTTPContextIsolatesRequestsAndPreservesCancellation(t *testing.T) {
	var logs bytes.Buffer
	var contexts []*server.HTTPContext
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := server.GetHTTPContext(r)
		contexts = append(contexts, ctx)
		if ctx.RequestStart.IsZero() || ctx.StatusCode != 0 || ctx.Err != nil {
			t.Fatalf("unexpected initial context: %+v", ctx)
		}
		if len(contexts) == 1 {
			if !errors.Is(r.Context().Err(), context.Canceled) {
				t.Fatal("middleware lost request cancellation")
			}
			ctx.Err = errors.New("storage failed")
			w.WriteHeader(500)
		}
	})
	h := server.HTTPContextMiddleware(server.LoggingMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), next))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil).WithContext(ctx))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if contexts[0] == contexts[1] || contexts[0].StatusCode != 500 || contexts[1].StatusCode != 200 {
		t.Fatalf("request state leaked: first=%+v second=%+v", contexts[0], contexts[1])
	}
	d := json.NewDecoder(&logs)
	for _, want := range []struct{ level, errorMessage string }{{"ERROR", "storage failed"}, {"INFO", ""}} {
		var got struct{ Level, Error string }
		if err := d.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Level != want.level || got.Error != want.errorMessage {
			t.Fatalf("unexpected request log: %+v", got)
		}
	}
}

func TestLoggingRecordsAbortedRequestWithoutLeakingPath(t *testing.T) {
	var logs bytes.Buffer
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) })
	h := server.HTTPContextMiddleware(server.LoggingMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), next))
	func() {
		defer func() {
			if got := recover(); got != http.ErrAbortHandler {
				t.Fatalf("panic changed: %v", got)
			}
		}()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/secret-path?secret-query", nil))
	}()
	var got struct {
		Status  int
		Aborted bool
	}
	if err := json.Unmarshal(logs.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Aborted || got.Status != 0 || strings.Contains(logs.String(), "secret") {
		t.Fatalf("unexpected aborted request log: %s", &logs)
	}
}
