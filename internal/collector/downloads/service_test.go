package downloads

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type archiveFunc func(context.Context, panda.GalleryRef) (string, error)

func (f archiveFunc) GetArchiveURL(ctx context.Context, ref panda.GalleryRef) (string, error) {
	return f(ctx, ref)
}

type transferFunc func(context.Context, string, io.Writer) (int64, error)

func (f transferFunc) Copy(ctx context.Context, url string, w io.Writer) (int64, error) {
	return f(ctx, url, w)
}

func zipBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create("page.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("image contents")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func openDB(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func startService(t *testing.T, db *sql.DB, dir string, client ArchiveClient, transfer Transfer) *Service {
	t.Helper()
	s, err := New(t.Context(), db, dir, client, transfer, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func waitJob(t *testing.T, s *Service, id int64, ready func(Job) bool) Job {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, err := s.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if ready(job) {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("download did not reach expected state: %+v", job)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDownloadPublicationDeduplicationRetentionAndDeletion(t *testing.T) {
	dbDir, downloadDir := t.TempDir(), t.TempDir()
	db := openDB(t, dbDir)
	archive := zipBytes(t)
	started, release := make(chan struct{}), make(chan struct{})
	var preparations atomic.Int64
	client := archiveFunc(func(context.Context, panda.GalleryRef) (string, error) {
		preparations.Add(1)
		return "archive-url", nil
	})
	transfer := transferFunc(func(ctx context.Context, _ string, w io.Writer) (int64, error) {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return io.Copy(w, bytes.NewReader(archive))
	})
	s := startService(t, db, downloadDir, client, transfer)
	ref := panda.GalleryRef{ID: 42, Token: "token"}
	if _, err := s.Submit(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("transfer did not start")
	}
	if _, err := s.Open(t.Context(), 42); !errors.Is(err, ErrState) {
		t.Fatalf("incomplete archive exposed: %v", err)
	}
	if job, err := s.Submit(t.Context(), ref); err != nil || job.State != "running" {
		t.Fatalf("duplicate: %+v, %v", job, err)
	}
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 42, Token: "conflicting"}); !errors.Is(err, ErrTokenConflict) {
		t.Fatalf("conflict: %v", err)
	}
	close(release)
	job := waitJob(t, s, 42, func(j Job) bool { return j.State == "completed" })
	if job.SizeBytes != int64(len(archive)) {
		t.Fatalf("size: %d", job.SizeBytes)
	}
	s.Close()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openDB(t, dbDir)
	s = startService(t, db, downloadDir, client, transfer)
	if job, err := s.Submit(t.Context(), ref); err != nil || job.State != "completed" {
		t.Fatalf("retained duplicate: %+v, %v", job, err)
	}
	for range 2 {
		file, err := s.Open(t.Context(), 42)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(file)
		file.Close()
		if err != nil || !bytes.Equal(body, archive) {
			t.Fatalf("retained contents: %v", err)
		}
	}
	if preparations.Load() != 1 {
		t.Fatalf("archive was fetched %d times", preparations.Load())
	}
	if err := s.Delete(t.Context(), 42); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(t.Context(), 42); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("job not deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(downloadDir, "[42].zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive not deleted: %v", err)
	}
	s.Close()
	if _, err := s.Submit(t.Context(), ref); err != nil {
		t.Fatalf("fresh submission after deletion: %v", err)
	}
}

func TestCancellationCleansPartialAndCanRetry(t *testing.T) {
	dir := t.TempDir()
	started := make(chan struct{}, 2)
	s := startService(t, openDB(t, t.TempDir()), dir,
		archiveFunc(func(context.Context, panda.GalleryRef) (string, error) { return "url", nil }),
		transferFunc(func(ctx context.Context, _ string, w io.Writer) (int64, error) {
			if _, err := w.Write([]byte("partial")); err != nil {
				return 0, err
			}
			started <- struct{}{}
			<-ctx.Done()
			return 7, ctx.Err()
		}))
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("transfer did not start")
		}
		job, err := s.Cancel(t.Context(), 1)
		if err != nil || job.State != "cancelled" {
			t.Fatalf("cancel: %+v, %v", job, err)
		}
		if _, err := os.Stat(filepath.Join(dir, "[1].zip.part")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("partial retained: %v", err)
		}
		if attempt == 0 {
			if _, err := s.Retry(t.Context(), 1); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestRestartRecoversPublicationAndUnfinishedJobs(t *testing.T) {
	dir := t.TempDir()
	db := openDB(t, t.TempDir())
	if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at) VALUES
		(1, 'token', 'running', 1, 1), (2, 'token', 'running', 2, 2),
		(3, 'token', 'deleting', 3, 3), (4, 'token', 'cancelled', 4, 4)`); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"[1].zip", "[2].zip.part", "[3].zip", "[4].zip.part"} {
		if err := os.WriteFile(filepath.Join(dir, name), zipBytes(t), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	started := make(chan int64, 1)
	s := startService(t, db, dir, archiveFunc(func(ctx context.Context, ref panda.GalleryRef) (string, error) {
		started <- ref.ID
		<-ctx.Done()
		return "", ctx.Err()
	}), nil)
	select {
	case id := <-started:
		if id != 2 {
			t.Fatalf("restarted gallery %d", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unfinished job did not restart")
	}
	if job, err := s.Get(t.Context(), 1); err != nil || job.State != "completed" {
		t.Fatalf("publication recovery: %+v, %v", job, err)
	}
	if _, err := s.Get(t.Context(), 3); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deletion recovery: %v", err)
	}
	for _, name := range []string{"[2].zip.part", "[3].zip", "[4].zip.part"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("recovery retained %s: %v", name, err)
		}
	}
}

func TestRetriesAreBoundedAndDelayedJobsDoNotBlockQueue(t *testing.T) {
	db := openDB(t, t.TempDir())
	var calls atomic.Int64
	s := startService(t, db, t.TempDir(), archiveFunc(func(_ context.Context, ref panda.GalleryRef) (string, error) {
		if ref.ID == 1 {
			calls.Add(1)
			return "", &panda.HTTPError{StatusCode: 503}
		}
		return "", &panda.BanError{Until: time.Now().Add(time.Hour)}
	}), nil)
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, s, 1, func(j Job) bool { return j.State == "queued" && j.Failures == 1 })
	if job.RetryAt == nil || !job.RetryAt.After(time.Now()) {
		t.Fatal("missing retry deadline")
	}
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 2, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	banned := waitJob(t, s, 2, func(j Job) bool { return j.Error == "panda_banned" })
	if banned.Failures != 0 || banned.State != "queued" || banned.RetryAt == nil {
		t.Fatalf("ban consumed retry budget: %+v", banned)
	}
	for failures := int64(2); failures <= maxFailures; failures++ {
		if _, err := db.Exec(`UPDATE panda_downloads SET retry_at = 0 WHERE gallery_id = 1`); err != nil {
			t.Fatal(err)
		}
		s.signal()
		job = waitJob(t, s, 1, func(j Job) bool { return j.Failures == failures })
	}
	if job.State != "failed" || calls.Load() != maxFailures {
		t.Fatalf("unbounded retries: %+v, calls=%d", job, calls.Load())
	}
	if duplicate, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil || duplicate.State != "failed" {
		t.Fatalf("submission retried failure: %+v, %v", duplicate, err)
	}
	s.Close()
	if job, err := s.Retry(t.Context(), 1); err != nil || job.State != "queued" || job.Failures != 0 {
		t.Fatalf("manual retry: %+v, %v", job, err)
	}
}

func TestInvalidZIPNeverPublished(t *testing.T) {
	s := startService(t, openDB(t, t.TempDir()), t.TempDir(),
		archiveFunc(func(context.Context, panda.GalleryRef) (string, error) { return "url", nil }),
		transferFunc(func(_ context.Context, _ string, w io.Writer) (int64, error) {
			return io.Copy(w, bytes.NewBufferString("<html>Unavailable</html>"))
		}))
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	waitJob(t, s, 1, func(j Job) bool { return j.Error == "invalid_zip" })
	if _, err := s.Open(t.Context(), 1); !errors.Is(err, ErrState) {
		t.Fatalf("invalid ZIP exposed: %v", err)
	}
	if _, err := os.Stat(s.path(1)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid ZIP published: %v", err)
	}
}

func TestExpiredURLTriggersFreshArchivePreparation(t *testing.T) {
	db := openDB(t, t.TempDir())
	archive := zipBytes(t)
	var calls atomic.Int64
	s := startService(t, db, t.TempDir(),
		archiveFunc(func(context.Context, panda.GalleryRef) (string, error) {
			if calls.Add(1) == 1 {
				return "expired", nil
			}
			return "fresh", nil
		}),
		transferFunc(func(_ context.Context, address string, w io.Writer) (int64, error) {
			if address == "expired" {
				return 0, errExpiredURL
			}
			return io.Copy(w, bytes.NewReader(archive))
		}))
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	waitJob(t, s, 1, func(j Job) bool { return j.Error == "archive_url_expired" })
	if _, err := db.Exec(`UPDATE panda_downloads SET retry_at = 0 WHERE gallery_id = 1`); err != nil {
		t.Fatal(err)
	}
	s.signal()
	waitJob(t, s, 1, func(j Job) bool { return j.State == "completed" })
	if calls.Load() != 2 {
		t.Fatalf("preparations: %d", calls.Load())
	}
}
