package server

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type uploadDeadlineRecorder struct {
	*httptest.ResponseRecorder
	read, write time.Time
}

func (w *uploadDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	w.read = deadline
	return nil
}

func (w *uploadDeadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.write = deadline
	return nil
}

type interruptedUpload struct{}

func (interruptedUpload) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestPrepareUpload(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   io.Reader
		length int64
		large  bool
		broken bool
	}{
		{"at limit", strings.NewReader("1234"), 4, false, false},
		{"known oversize", strings.NewReader("12345"), 5, true, false},
		{"streamed oversize", strings.NewReader("12345"), -1, true, false},
		{"interrupted", interruptedUpload{}, -1, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/upload", tc.body)
			r.ContentLength = tc.length
			w := &uploadDeadlineRecorder{ResponseRecorder: httptest.NewRecorder(), read: time.Now(), write: time.Now()}
			err := PrepareUpload(w, r, 4)
			if err == nil {
				if !w.read.IsZero() || !w.write.IsZero() {
					t.Fatal("upload retained ordinary API deadlines")
				}
				_, err = io.Copy(io.Discard, r.Body)
			}
			_, large := errors.AsType[*http.MaxBytesError](err)
			if large != tc.large || (!tc.large && errors.Is(err, ErrUploadInterrupted) != tc.broken) {
				t.Fatalf("read error = %v", err)
			}
			if tc.broken && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("original read failure lost: %v", err)
			}
			if !tc.large && !tc.broken && err != nil {
				t.Fatal(err)
			}
		})
	}
}
