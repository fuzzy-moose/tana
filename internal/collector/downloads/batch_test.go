package downloads

import (
	"errors"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/downloads/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestSubmitBatchReusesEveryStateAndDeduplicates(t *testing.T) {
	db := openDB(t, t.TempDir())
	s := &Service{db: db, q: dbgen.New(db), wake: make(chan struct{}, 1)}
	states := []string{"queued", "running", "completed", "failed", "cancelled", "deleting"}
	refs := []panda.GalleryRef{{ID: 99, Token: "new"}, {ID: 99, Token: "new"}}
	for i, state := range states {
		id := int64(i + 1)
		if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at, retry_at, failures)
			VALUES (?, 'token', ?, 1, 2, 3, 4)`, id, state); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, panda.GalleryRef{ID: id, Token: "token"}, panda.GalleryRef{ID: id, Token: "token"})
	}
	got, err := s.SubmitBatch(t.Context(), refs)
	want := collectorapi.DownloadBatchCounts{NewDownloads: 1, ExistingJobs: 2, RetainedArchives: 1, Failed: 1, Cancelled: 1, Deleting: 1}
	if err != nil || got != want {
		t.Fatalf("submit=%+v %v, want %+v", got, err, want)
	}
	for i, state := range states {
		job, err := s.q.GetDownload(t.Context(), int64(i+1))
		if err != nil || job.State != state || job.UpdatedAt != 2 || job.RetryAt != 3 || job.Failures != 4 {
			t.Fatalf("existing job changed: %+v %v", job, err)
		}
	}
	got, err = s.SubmitBatch(t.Context(), refs)
	want.NewDownloads, want.ExistingJobs = 0, 3
	if err != nil || got != want {
		t.Fatalf("resubmit=%+v %v, want %+v", got, err, want)
	}
}

func TestSubmitBatchRollsBackEveryRequestOnError(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []panda.GalleryRef
		want error
	}{
		{"invalid", []panda.GalleryRef{{ID: 2, Token: "new"}, {ID: 3, Token: " "}}, ErrInvalidReference},
		{"existing token conflict", []panda.GalleryRef{{ID: 2, Token: "new"}, {ID: 1, Token: "conflict"}}, ErrTokenConflict},
		{"duplicate token conflict", []panda.GalleryRef{{ID: 2, Token: "new"}, {ID: 2, Token: "conflict"}}, ErrTokenConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openDB(t, t.TempDir())
			s := &Service{db: db, q: dbgen.New(db), wake: make(chan struct{}, 1)}
			if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "original"}); err != nil {
				t.Fatal(err)
			}
			if got, err := s.SubmitBatch(t.Context(), tc.refs); !errors.Is(err, tc.want) || got != (collectorapi.DownloadBatchCounts{}) {
				t.Fatalf("submit=%+v %v, want %v", got, err, tc.want)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM panda_downloads`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("partial batch committed: count=%d %v", count, err)
			}
			row, err := s.q.GetDownload(t.Context(), 1)
			if err != nil || row.Token != "original" {
				t.Fatalf("existing job changed: %+v %v", row, err)
			}
		})
	}
}
