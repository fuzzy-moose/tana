package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestShutdownDrainsActiveRequest(t *testing.T) {
	testShutdown(t, false)
}

func TestShutdownDeadlineClosesActiveRequest(t *testing.T) {
	testShutdown(t, true)
}

func testShutdown(t *testing.T, force bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	handlerDone := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		close(started)
		if force {
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		<-ctx.Done()
		_, _ = w.Write([]byte("finished"))
	})
	grace := 2 * time.Second
	if force {
		grace = 20 * time.Millisecond
	}
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, listener, Config{GracePeriod: grace}, slog.New(slog.NewJSONHandler(io.Discard, nil)), handler)
	}()
	clientDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if err != nil {
			clientDone <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err == nil && string(body) != "finished" {
			err = errors.New("incomplete response")
		}
		clientDone <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if force && !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline, got %v", err)
		}
		if !force && err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	select {
	case err := <-clientDone:
		if !force && err != nil {
			t.Fatal(err)
		}
		if force && err == nil {
			t.Fatal("expected interrupted response")
		}
	case <-time.After(4 * time.Second):
		t.Fatal("client did not finish")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish")
	}
}
