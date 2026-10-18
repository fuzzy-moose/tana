package collectorapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestFavoriteDownloadCandidatesSupportsWholeCategory(t *testing.T) {
	c, err := NewClient("https://collector.test/base", "secret")
	if err != nil {
		t.Fatal(err)
	}
	want := make([]FavoriteDownloadCandidate, 20000)
	for i := range want {
		want[i] = FavoriteDownloadCandidate{Ref: panda.GalleryRef{ID: int64(i + 1), Token: "123456789a"}, State: "completed"}
	}
	body, err := json.Marshal(struct {
		Favorites []FavoriteDownloadCandidate `json:"favorites"`
	}{want})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 1<<20 {
		t.Fatal("fixture must exceed ordinary download response limit")
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/base/api/favorites/2/download-candidates" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("request: %s %s %v", r.Method, r.URL, r.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	got, err := c.FavoriteDownloadCandidates(t.Context(), 2)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates=%d %v", len(got), err)
	}
}

func TestSubmitDownloadsUsesOneBatchRequest(t *testing.T) {
	c, err := NewClient("https://collector.test", "secret")
	if err != nil {
		t.Fatal(err)
	}
	refs := []panda.GalleryRef{{ID: 42, Token: "gallery-token"}, {ID: 43, Token: "other-token"}}
	want := DownloadBatchCounts{NewDownloads: 1, Failed: 1}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var input struct {
			References []panda.GalleryRef `json:"references"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatal(err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/downloads/batch" || r.Header.Get("Authorization") != "Bearer secret" || !reflect.DeepEqual(input.References, refs) {
			t.Fatalf("batch request: %s %s %+v", r.Method, r.URL, input)
		}
		body, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	got, err := c.SubmitDownloads(t.Context(), refs)
	if err != nil || got != want {
		t.Fatalf("submit=%+v %v", got, err)
	}
}
