package panda_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}
}

const testAPIURL = "https://panda.example.test/custom/api.php?mode=metadata"

func newTestClient(t *testing.T, httpClient *http.Client) *panda.Client {
	t.Helper()
	client, err := panda.NewClient(testAPIURL, httpClient)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestGetMetadata(t *testing.T) {
	client := newTestClient(t, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != testAPIURL {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected content type: %v", r.Header)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		want := map[string]any{
			"method": "gdata", "namespace": float64(1),
			"gidlist": []any{[]any{float64(123), "abc"}, []any{float64(456), "bad"}},
		}
		if !reflect.DeepEqual(request, want) {
			t.Fatalf("request = %#v, want %#v", request, want)
		}
		return jsonResponse(`{"gmetadata":[{
			"gid":123,"token":"abc","title":"Title","title_jpn":"題名",
			"category":"Manga","thumb":"https://example.com/thumb.jpg","uploader":"artist",
			"posted":"1653702810","filecount":"329","filesize":419547090,
			"expunged":true,"rating":"4.68","torrentcount":"1",
			"torrents":[{"hash":"hash","added":"1634958428","name":"archive.zip","tsize":"12256","fsize":"5310511523"}],
			"tags":["artist:someone","other:Unmodified Tag"],
			"parent_gid":"122","parent_key":"parent","current_gid":"124","current_key":"current",
			"first_gid":"100","first_key":"first"
		},{"gid":456,"error":"Key missing, or incorrect key provided."}]}`), nil
	})})
	got, err := client.GetMetadata(t.Context(), []panda.GalleryRef{{ID: 123, Token: "abc"}, {ID: 456, Token: "bad"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []panda.Metadata{
		{
			ID: 123, Token: "abc", Title: "Title", TitleJapanese: "題名", Category: "Manga",
			ThumbnailURL: "https://example.com/thumb.jpg", Uploader: "artist", Posted: 1653702810,
			FileCount: 329, FileSize: 419547090, Expunged: true, Rating: 4.68, TorrentCount: 1,
			Torrents: []panda.Torrent{{Hash: "hash", Added: 1634958428, Name: "archive.zip", TorrentSize: 12256, FileSize: 5310511523}},
			Tags:     []string{"artist:someone", "other:Unmodified Tag"},
			ParentID: 122, ParentToken: "parent", CurrentID: 124, CurrentToken: "current", FirstID: 100, FirstToken: "first",
		},
		{ID: 456, Error: "Key missing, or incorrect key provided."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
}

func TestGetMetadataBatchValidation(t *testing.T) {
	calls := 0
	client := newTestClient(t, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		var request struct {
			Galleries [][2]json.RawMessage `json:"gidlist"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Galleries) != 25 {
			t.Fatalf("batch size = %d, want 25", len(request.Galleries))
		}
		entries := make([]string, 25)
		for i := range entries {
			entries[i] = `{"gid":1,"token":"abc"}`
		}
		return jsonResponse(`{"gmetadata":[` + strings.Join(entries, ",") + `]}`), nil
	})})
	for _, input := range [][]panda.GalleryRef{
		nil,
		make([]panda.GalleryRef, 26),
		{{ID: 0, Token: "abc"}},
		{{ID: -1, Token: "abc"}},
		{{ID: 1, Token: ""}},
	} {
		if _, err := client.GetMetadata(t.Context(), input); err == nil {
			t.Fatalf("expected validation error for %+v", input)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid batches made %d requests", calls)
	}
	input := make([]panda.GalleryRef, 25)
	for i := range input {
		input[i] = panda.GalleryRef{ID: 1, Token: "abc"}
	}
	got, err := client.GetMetadata(t.Context(), input)
	if err != nil || len(got) != 25 || calls != 1 {
		t.Fatalf("maximum batch: %d results, %d requests, error %v", len(got), calls, err)
	}
}

func TestGetMetadataResponseFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"rate limited", 429, "slow down"},
		{"server error", 503, "unavailable"},
		{"invalid JSON", 200, "<html>error</html>"},
		{"invalid numeric field", 200, `{"gmetadata":[{"gid":1,"rating":"invalid"}]}`},
		{"API error", 200, `{"error":"unavailable"}`},
		{"missing metadata", 200, `{}`},
		{"null metadata", 200, `{"gmetadata":null}`},
		{"incomplete batch", 200, `{"gmetadata":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			body := &trackedBody{Reader: strings.NewReader(tc.body)}
			client := newTestClient(t, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": {"5"}}, Body: body}, nil
			})})
			got, err := client.GetMetadata(t.Context(), []panda.GalleryRef{{ID: 1, Token: "abc"}})
			if err == nil || got != nil {
				t.Fatalf("expected batch failure, got %+v, %v", got, err)
			}
			if calls != 1 || !body.closed {
				t.Fatalf("requests = %d, body closed = %v", calls, body.closed)
			}
			if tc.status != 200 {
				var statusErr *panda.HTTPError
				if !errors.As(err, &statusErr) || statusErr.StatusCode != tc.status || statusErr.RetryAfter != "5" {
					t.Fatalf("missing HTTP failure details: %v", err)
				}
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

func TestGetMetadataCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	client := newTestClient(t, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		cancel()
		return nil, r.Context().Err()
	})})
	_, err := client.GetMetadata(ctx, []panda.GalleryRef{{ID: 1, Token: "abc"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
