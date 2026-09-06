package metadata

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func openDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedRefs(t *testing.T, db *sql.DB, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, fmt.Sprintf("token%d", id)); err != nil {
			t.Fatal(err)
		}
	}
}

func apiClient(t *testing.T, transport http.RoundTripper) *panda.Client {
	t.Helper()
	client, err := panda.NewClient("https://panda.example.test/api", &http.Client{Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func requestedIDs(t *testing.T, req *http.Request) []int64 {
	t.Helper()
	var body struct {
		GIDList [][2]json.RawMessage `json:"gidlist"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, pair := range body.GIDList {
		var id int64
		if err := json.Unmarshal(pair[0], &id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func metadataResponse(t *testing.T, entries ...panda.Metadata) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]any{"gmetadata": entries})
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(body)))}
}

func TestCollectsRelatedGalleriesAndRetainsMetadataAcrossRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openDB(t, dir)
		seedRefs(t, db, 1)
		want := panda.Metadata{
			ID: 1, Token: "token1", Title: "Original Title", TitleJapanese: "原題", Category: "Manga",
			ThumbnailURL: "https://example.test/thumb.jpg", Uploader: "Uploader", Posted: 1700000000,
			FileCount: 42, FileSize: 123456, Expunged: true, Rating: 4.5, TorrentCount: 1,
			Torrents: []panda.Torrent{{Hash: "hash", Added: 1700000001, Name: "Release", TorrentSize: 123, FileSize: 456}},
			Tags:     []string{"artist:Mixed Case", "unprefixed"},
			ParentID: 2, ParentToken: "token2", CurrentID: 3, CurrentToken: "token3", FirstID: 4, FirstToken: "token4",
		}
		var requested []int64
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			ids := requestedIDs(t, req)
			requested = append(requested, ids...)
			var entries []panda.Metadata
			// Upstream order need not match request order.
			for _, id := range slices.Backward(ids) {
				entry := panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id), ParentID: 1, ParentToken: "token1"}
				if id == 1 {
					entry = want
				}
				entries = append(entries, entry)
			}
			return metadataResponse(t, entries...), nil
		}))
		at := time.Now().UTC()
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if !reflect.DeepEqual(requested, []int64{1, 2, 3, 4}) {
			t.Fatalf("requested galleries = %v", requested)
		}
		s.Close()
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDB(t, dir)
		s = New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		time.Sleep(10 * time.Minute)
		synctest.Wait()
		got, err := s.Get(t.Context(), 1)
		if err != nil || !reflect.DeepEqual(got.Metadata, want) || !got.RefreshedAt.Equal(at) {
			t.Fatalf("retained metadata = %+v, %v; want %+v at %s", got, err, want, at)
		}
		if len(requested) != 4 {
			t.Fatalf("successful galleries fetched again: %v", requested)
		}
	})
}

func TestRequestBackoffSurvivesRestartAndGalleryErrorsDoNotBlockCollection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		db := openDB(t, dir)
		seedRefs(t, db, 1, 2)
		var batches [][]int64
		var batchMu sync.Mutex
		getBatches := func() [][]int64 {
			batchMu.Lock()
			defer batchMu.Unlock()
			return slices.Clone(batches)
		}
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			ids := requestedIDs(t, req)
			batchMu.Lock()
			defer batchMu.Unlock()
			batches = append(batches, ids)
			if len(batches) == 1 {
				return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"120"}}, Body: io.NopCloser(strings.NewReader("busy"))}, nil
			}
			var entries []panda.Metadata
			for _, id := range ids {
				entry := panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id), Title: "Saved"}
				if id == 1 {
					entry = panda.Metadata{ID: 1, Error: "Gallery not found"}
				}
				entries = append(entries, entry)
			}
			return metadataResponse(t, entries...), nil
		}))
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		s.Close()
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDB(t, dir)
		// New discoveries must not bypass the API-wide Retry-After delay.
		seedRefs(t, db, 3)
		s = New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		time.Sleep(119 * time.Second)
		synctest.Wait()
		if got := getBatches(); len(got) != 1 {
			t.Fatalf("retried before Retry-After expired: %v", got)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := getBatches(); !reflect.DeepEqual(got, [][]int64{{1, 2}, {1, 2, 3}}) {
			t.Fatalf("pending work after restart = %v", got)
		}
		got, err := s.Get(t.Context(), 2)
		if err != nil || got.Metadata.Title != "Saved" || !got.RefreshedAt.Equal(time.Now()) {
			t.Fatalf("successful gallery in mixed batch = %+v, %v", got, err)
		}
		if _, err := s.Get(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("gallery failure stored as metadata: %v", err)
		}
		s.Close()
		s = New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if got := getBatches(); len(got) != 2 {
			t.Fatalf("parked or successful gallery retried: %v", got)
		}
	})
}

func TestDrainsFullBatchesAndPicksUpNewReferences(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openDB(t, t.TempDir())
		for id := int64(1); id <= 26; id++ {
			seedRefs(t, db, id)
		}
		var sizes []int
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			ids := requestedIDs(t, req)
			sizes = append(sizes, len(ids))
			var entries []panda.Metadata
			for _, id := range ids {
				entries = append(entries, panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)})
			}
			return metadataResponse(t, entries...), nil
		}))
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		seedRefs(t, db, 27)
		time.Sleep(time.Second)
		synctest.Wait()
		if !reflect.DeepEqual(sizes, []int{25, 1, 1}) {
			t.Fatalf("batch sizes = %v", sizes)
		}
		if got, err := s.Get(t.Context(), 27); err != nil || got.Metadata.ID != 27 {
			t.Fatalf("new reference was not collected: %+v, %v", got, err)
		}
	})
}

func TestMetadataAndRelatedDiscoveryCommitTogether(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := openDB(t, t.TempDir())
		seedRefs(t, db, 1)
		if _, err := db.Exec(`CREATE TRIGGER reject_discovery BEFORE INSERT ON gallery_refs
			WHEN NEW.gallery_id = 2 BEGIN SELECT RAISE(FAIL, 'storage failure'); END`); err != nil {
			t.Fatal(err)
		}
		var requested []int64
		client := apiClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			ids := requestedIDs(t, req)
			requested = append(requested, ids...)
			if ids[0] == 1 {
				return metadataResponse(t, panda.Metadata{ID: 1, Token: "token1", ParentID: 2, ParentToken: "token2"}), nil
			}
			return metadataResponse(t, panda.Metadata{ID: 2, Token: "token2"}), nil
		}))
		s := New(t.Context(), db, client, slog.New(slog.DiscardHandler))
		defer s.Close()
		synctest.Wait()
		if _, err := s.Get(t.Context(), 1); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("metadata committed despite lost discovery: %v", err)
		}
		if _, err := db.Exec(`DROP TRIGGER reject_discovery`); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		if !reflect.DeepEqual(requested, []int64{1, 1, 2}) {
			t.Fatalf("discovery did not recover: %v", requested)
		}
		for _, id := range []int64{1, 2} {
			if _, err := s.Get(t.Context(), id); err != nil {
				t.Fatal(err)
			}
		}
	})
}
