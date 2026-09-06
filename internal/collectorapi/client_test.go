package collectorapi

import (
	"encoding/json"
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
