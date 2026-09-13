package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type cancelAtEOF struct {
	io.Reader
	cancel context.CancelFunc
}

func (r cancelAtEOF) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == io.EOF {
		r.cancel()
	}
	return n, err
}

func TestDownloadBatchReceivesCompleteBodyBeforeDurableAcceptance(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := slog.New(slog.DiscardHandler)
	service, err := downloads.New(t.Context(), db, t.TempDir(), waitingArchiveClient{}, nil, logger, downloads.StorageConfig{PauseBelowBytes: 1, ResumeAtBytes: 2})
	if err != nil {
		t.Fatal(err)
	}
	service.Close()
	handler := NewHandler(&collector.App{Logger: logger, Downloads: service, APIToken: "test-token"})
	for _, body := range []string{
		`{"references":[{"gid":1,"token":"secret"}]} {}`,
		`{"references":[{"gid":1,"token":"secret"}]`,
		`{"references":[{"gid":1,"token":"secret"},{"gid":2,"token":""}]}`,
		`{}`,
	} {
		w := apiRequest(handler, http.MethodPost, "/api/downloads/batch", body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("invalid batch accepted: %d %s", w.Code, w.Body)
		}
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM panda_downloads`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("invalid body committed jobs: %d %v", count, err)
		}
	}
	refs := make([]panda.GalleryRef, 1000)
	for i := range refs {
		refs[i] = panda.GalleryRef{ID: int64(i + 1), Token: "secret"}
	}
	body, err := json.Marshal(struct {
		References []panda.GalleryRef `json:"references"`
	}{refs})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	r := httptest.NewRequest(http.MethodPost, "/api/downloads/batch", cancelAtEOF{Reader: strings.NewReader(string(body)), cancel: cancel}).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer test-token")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var result collectorapi.DownloadBatchCounts
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != http.StatusAccepted || result.NewDownloads != 1000 {
		t.Fatalf("complete body did not survive disconnect: %d %s %v", w.Code, w.Body, err)
	}
	if ctx.Err() == nil {
		t.Fatal("request was not cancelled")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM panda_downloads`).Scan(&count); err != nil || count != 1000 {
		t.Fatalf("batch not committed: %d %v", count, err)
	}
}
