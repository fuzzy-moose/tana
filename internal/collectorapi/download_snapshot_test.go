package collectorapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestCompletedDownloadIDsSupportsWholeBacklog(t *testing.T) {
	c, err := NewClient("https://collector.test/base", "secret")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]int64, 200000)
	for i := range want {
		want[i] = int64(len(want) - i)
	}
	body, err := json.Marshal(struct {
		GalleryIDs []int64 `json:"gallery_ids"`
	}{want})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 1<<20 {
		t.Fatal("fixture must exceed ordinary download response limit")
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/base/api/downloads/completed" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request: %s %s %v", r.Method, r.URL, r.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	got, err := c.CompletedDownloadIDs(t.Context())
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("snapshot count=%d, %v", len(got), err)
	}
}

func TestCompletedDownloadIDsRejectsInvalidSnapshots(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`, `{"gallery_ids":null}`, `{"gallery_ids":[0]}`,
		`{"gallery_ids":[-1]}`, `{"gallery_ids":[1,1]}`,
		`{"gallery_ids":[1]`, `{"gallery_ids":[1]} {}`,
	} {
		t.Run(body, func(t *testing.T) {
			c, err := NewClient("https://collector.test", "secret")
			if err != nil {
				t.Fatal(err)
			}
			c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			if ids, err := c.CompletedDownloadIDs(t.Context()); err == nil || ids != nil {
				t.Fatalf("invalid snapshot accepted: %v, %v", ids, err)
			}
		})
	}
}

func TestCompletedDownloadIDsAcceptsEmptySnapshotAndPropagatesHTTPError(t *testing.T) {
	c, err := NewClient("https://collector.test", "secret")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"gallery_ids":[]}`))}, nil
	})
	if ids, err := c.CompletedDownloadIDs(t.Context()); err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("empty snapshot: %v, %v", ids, err)
	}
	c.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`))}, nil
	})
	_, err = c.CompletedDownloadIDs(t.Context())
	if responseErr, ok := errors.AsType[*HTTPError](err); !ok || responseErr.StatusCode != http.StatusUnauthorized || responseErr.Code != "unauthorized" {
		t.Fatalf("HTTP error: %v", err)
	}
}
