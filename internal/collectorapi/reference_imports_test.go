package collectorapi

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type observedImportReader struct {
	reader io.Reader
	reads  int
}

func (r *observedImportReader) Read(p []byte) (int, error) {
	r.reads++
	return r.reader.Read(p)
}

func TestReferenceImportUploadStreamsThroughTransferClient(t *testing.T) {
	client, err := NewClient("https://collector.test/base", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if client.files.Timeout != 0 {
		t.Fatal("upload client has a total transfer timeout")
	}
	const input = "42,reference-secret\n"
	body := &observedImportReader{reader: strings.NewReader(input)}
	client.http.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("upload used the ordinary API client")
		return nil, nil
	})
	client.files.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if body.reads != 0 {
			t.Fatal("file read before the transfer started")
		}
		if r.Method != "POST" || r.URL.Path != "/base/api/reference-imports" || r.URL.Query().Get("filename") != "日本 + #&.txt" ||
			r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Fatalf("upload request: %s %s %v", r.Method, r.URL, r.Header)
		}
		got, err := io.ReadAll(r.Body)
		if err != nil || string(got) != input {
			t.Fatalf("upload body = %q, %v", got, err)
		}
		return &http.Response{StatusCode: 202, Body: io.NopCloser(strings.NewReader(`{"id":"accepted","filename":"日本 + #&.txt","status":"processing"}`))}, nil
	})
	result, err := client.SubmitReferenceImport(t.Context(), "日本 + #&.txt", body)
	if err != nil || result.ID != "accepted" {
		t.Fatalf("result = %+v, %v", result, err)
	}
	client.files.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 413, Body: io.NopCloser(strings.NewReader(`{"error":"import_too_large"}`))}, nil
	})
	_, err = client.SubmitReferenceImport(t.Context(), "too-large.txt", strings.NewReader(""))
	upstream, ok := errors.AsType[*HTTPError](err)
	if !ok || upstream.StatusCode != 413 || upstream.Code != "import_too_large" {
		t.Fatalf("size rejection = %v", err)
	}
}
