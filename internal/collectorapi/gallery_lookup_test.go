package collectorapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestGalleryLookupAuthenticatesAndKeepsMissingDistinctFromUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   string
		valid  bool
	}{
		{"known", 200, `{"gid":42,"token":"known","url":"https://panda.test/g/42/known/"}`, "", true},
		{"fallback", 202, `{"gid":42,"token":"pasted","url":"https://panda.test/g/42/pasted/","unverified":true,"fetch_job":{"id":"job","status":"pending","created_at":"2026-09-13T00:00:00Z","entries":[{"gid":42,"status":"pending"}]}}`, "", true},
		{"missing", 404, `{"error":"gallery_not_found"}`, "gallery_not_found", false},
		{"outage", 503, `{"error":"internal_error"}`, "internal_error", false},
		{"wrong gallery", 200, `{"gid":43,"token":"known","url":"https://panda.test/g/43/known/"}`, "", false},
		{"incomplete", 200, `{"gid":42}`, "", false},
		{"fallback without job", 202, `{"gid":42,"token":"pasted","url":"https://panda.test/g/42/pasted/","unverified":true}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient("https://collector.test/base/", "secret")
			if err != nil {
				t.Fatal(err)
			}
			ref := panda.GalleryRef{ID: 42, Token: "pasted"}
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var got panda.GalleryRef
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got != ref ||
					r.Method != http.MethodPost || r.URL.Path != "/base/api/catalog/lookup" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Fatalf("lookup request: %s %s %+v, %v", r.Method, r.URL.Path, got, err)
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result, err := client.LookupGallery(t.Context(), ref)
			if (err == nil) != tc.valid {
				t.Fatalf("lookup = %+v, %v", result, err)
			}
			if tc.code != "" {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != tc.status || httpErr.Code != tc.code {
					t.Fatalf("lookup failure = %v", err)
				}
			}
		})
	}
}

func TestGetMetadataFetchReturnsDurableOutcome(t *testing.T) {
	client, err := NewClient("https://collector.test", "secret")
	if err != nil {
		t.Fatal(err)
	}
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/metadata/fetches/job" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("poll request: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
			`{"id":"job","status":"completed","created_at":"2026-09-13T00:00:00Z","completed_at":"2026-09-13T00:00:01Z","entries":[{"gid":42,"status":"failed","error":"token_mismatch"}]}`))}, nil
	})
	job, err := client.GetMetadataFetch(t.Context(), "job")
	if err != nil || job.Status != "completed" || job.CompletedAt == nil || len(job.Entries) != 1 || job.Entries[0].Error != "token_mismatch" {
		t.Fatalf("poll result = %+v, %v", job, err)
	}
}
