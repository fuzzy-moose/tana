package collectorapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenFeedCaptureHasNoTransferTimeout(t *testing.T) {
	client, err := NewClient("https://collector.test/base", "secret")
	if err != nil {
		t.Fatal(err)
	}
	const raw = "<feed>original bytes</feed>\r\n"
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if _, ok := r.Context().Deadline(); ok {
			t.Fatal("raw feed download has a total transfer timeout")
		}
		if r.Method != http.MethodGet || r.URL.Path != "/base/api/feed/captures/42/file" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("download request: %s %s %v", r.Method, r.URL, r.Header)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})
	client.http.Transport = transport
	client.files.Transport = transport
	response, err := client.OpenFeedCapture(t.Context(), 42, http.MethodGet)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != raw {
		t.Fatalf("download body = %q, %v", body, err)
	}
}
