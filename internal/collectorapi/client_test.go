package collectorapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLookupAuthenticatesAndRequiresCompleteDisjointOutcomes(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		valid      bool
	}{
		{"states", `{"galleries":[{"metadata":{"gid":1,"title":"API"}}],"pending_ids":[2],"failed_ids":[3],"unknown_ids":[4]}`, 200, true},
		{"omitted", `{"unknown_ids":[1,2,3]}`, 200, false},
		{"duplicate", `{"pending_ids":[1,2],"unknown_ids":[2,3,4]}`, 200, false},
		{"unrelated", `{"unknown_ids":[1,2,3,5]}`, 200, false},
		{"failed metadata", `{"galleries":[{"metadata":{"gid":1,"error":"failed"}}],"unknown_ids":[2,3,4]}`, 200, false},
		{"outage", `unavailable`, 503, false},
		{"malformed", `{`, 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient("https://collector.test/base/", "secret")
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body struct {
					IDs []int64 `json:"gallery_ids"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if r.URL.Path != "/base/api/metadata/lookup" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "application/json" || !reflect.DeepEqual(body.IDs, []int64{1, 2, 3, 4}) {
					t.Fatalf("unexpected request: %s %s, IDs %v", r.Method, r.URL.Path, body.IDs)
				}
				return &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(tt.body))}, nil
			})
			result, err := client.Lookup(t.Context(), []int64{1, 2, 3, 4})
			if (err == nil) != tt.valid {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestStatusReportsConnectionAndProtocolFailures(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status           int
		reachable        bool
	}{
		{"offline", "", "collector_unreachable", 0, false},
		{"older collector", "", "collector_status_unavailable", 404, true},
		{"redirect", "", "collector_status_unavailable", 302, true},
		{"broken", "{", "collector_invalid_response", 200, true},
		{"health instead of status", `{"status":"ok"}`, "collector_invalid_response", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewClient("https://collector.test/base/", "secret")
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.Path != "/base/api/status" || r.Header.Get("Authorization") != "Bearer secret" {
					t.Fatalf("status request: %s %s", r.Method, r.URL.Path)
				}
				if tc.status == 0 {
					return nil, errors.New("offline")
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			result := client.Status(t.Context())
			if result.Error != tc.code || result.Reachable != tc.reachable || result.Status != nil || result.Authenticated != nil {
				t.Fatalf("status: %+v", result)
			}
		})
	}
}

func TestFocusedStatusRequiresCompleteResponses(t *testing.T) {
	categories := make([]FavoriteCategory, 10)
	for i := range categories {
		categories[i].Category = i
	}
	valid, err := json.Marshal(FavoritesStatus{Categories: categories})
	if err != nil {
		t.Fatal(err)
	}
	categories[9].Category = 8
	duplicate, err := json.Marshal(FavoritesStatus{Categories: categories})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body string
		valid      bool
	}{
		{"favorites", string(valid), true},
		{"favorites", string(duplicate), false},
		{"favorites", `{}`, false},
		{"metadata", `{"metadata_errors":[]}`, true},
		{"metadata", `{}`, false},
	} {
		t.Run(tc.path+tc.body, func(t *testing.T) {
			client, err := NewClient("https://collector.test", "secret")
			if err != nil {
				t.Fatal(err)
			}
			client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path != "/api/"+tc.path+"/status" {
					t.Fatalf("unexpected status path: %s", r.URL.Path)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			if tc.path == "favorites" {
				_, err = client.FavoritesStatus(t.Context())
			} else {
				_, err = client.MetadataStatus(t.Context())
			}
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, error=%v", tc.valid, err)
			}
		})
	}
}
