package downloads

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type sizedTransfer struct {
	size int64
	copy transferFunc
}

func (f sizedTransfer) Copy(ctx context.Context, address string, w io.Writer, admit func(int64) error) (int64, error) {
	if err := admit(f.size); err != nil {
		return 0, err
	}
	return f.copy(ctx, address, w)
}

func storageService(t *testing.T, db *sql.DB, dir string, transfer Transfer, space func(string) (int64, error)) *Service {
	t.Helper()
	s, err := newService(t.Context(), db, dir,
		archiveFunc(func(context.Context, panda.GalleryRef) (string, error) { return "archive", nil }),
		transfer, slog.New(slog.DiscardHandler), StorageConfig{PauseBelowBytes: 50, ResumeAtBytes: 100}, space)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func downloadStorage(t *testing.T, s *Service) StorageStatus {
	t.Helper()
	list, err := s.List(t.Context(), "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	return list.Storage
}

func TestArchiveAdmissionWaitsForFullSizeAcrossRestart(t *testing.T) {
	db, dir := openDB(t, t.TempDir()), t.TempDir()
	archive := zipBytes(t)
	fullSize := int64(len(archive))
	var available, copies atomic.Int64
	available.Store(fullSize + 49)
	space := func(path string) (int64, error) {
		if path != dir {
			t.Errorf("checking %q instead of archive directory %q", path, dir)
		}
		return available.Load(), nil
	}
	transfer := sizedTransfer{fullSize, func(_ context.Context, _ string, w io.Writer) (int64, error) {
		copies.Add(1)
		return io.Copy(w, bytes.NewReader(archive))
	}}
	s := storageService(t, db, dir, transfer, space)
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 1, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, s, 1, func(j Job) bool { return j.State == "queued" && j.ExpectedSizeBytes == fullSize })
	if status := downloadStorage(t, s); !status.Paused || status.ResumeAtBytes != fullSize+100 || job.Failures != 0 || copies.Load() != 0 {
		t.Fatalf("archive did not wait before writing: %+v, %+v, copies=%d", status, job, copies.Load())
	}
	if _, err := os.Stat(s.path(1) + ".part"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("admission retained a partial: %v", err)
	}
	if _, err := s.Submit(t.Context(), panda.GalleryRef{ID: 2, Token: "token"}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	available.Store(fullSize + 99)
	s = storageService(t, db, dir, transfer, space)
	if status := downloadStorage(t, s); !status.Paused || status.ResumeAtBytes != fullSize+100 {
		t.Fatalf("restart bypassed recovery threshold: %+v", status)
	}
	if job, err := s.Get(t.Context(), 2); err != nil || job.State != "queued" || copies.Load() != 0 {
		t.Fatalf("later job bypassed pause: %+v, %v", job, err)
	}
	available.Store(fullSize + 100)
	s.signal()
	waitJob(t, s, 1, func(j Job) bool { return j.State == "completed" })
	waitJob(t, s, 2, func(j Job) bool { return j.State == "completed" })
	if copies.Load() != 2 || downloadStorage(t, s).Paused {
		t.Fatal("queue did not recover automatically")
	}
}

func TestStorageMonitoringInterruptsWithoutConsumingRetries(t *testing.T) {
	for _, tc := range []struct {
		name       string
		knownSize  bool
		checkFails bool
	}{
		{"known size, low space", true, false},
		{"unknown size, low space", false, false},
		{"known size, unavailable space", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, dir := openDB(t, t.TempDir()), t.TempDir()
			archive := zipBytes(t)
			partial := []byte("partial")
			expectedSize, reserve := int64(-1), int64(len(partial))
			if tc.knownSize {
				expectedSize, reserve = int64(len(archive)), int64(len(archive))
			}
			var available, copies atomic.Int64
			var checkFails atomic.Bool
			available.Store(1000)
			space := func(string) (int64, error) {
				if checkFails.Load() {
					return 0, errors.New("storage unavailable")
				}
				return available.Load(), nil
			}
			started := make(chan struct{})
			transfer := sizedTransfer{expectedSize, func(ctx context.Context, _ string, w io.Writer) (int64, error) {
				if copies.Add(1) == 1 {
					n, err := w.Write(partial)
					if err != nil {
						return int64(n), err
					}
					close(started)
					<-ctx.Done()
					return int64(n), ctx.Err()
				}
				return io.Copy(w, bytes.NewReader(archive))
			}}
			if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at, failures)
				VALUES (1, 'token', 'queued', 1, 1, 2)`); err != nil {
				t.Fatal(err)
			}
			s := storageService(t, db, dir, transfer, space)
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("transfer did not start")
			}
			if tc.checkFails {
				checkFails.Store(true)
			} else {
				available.Store(49)
			}
			job := waitJob(t, s, 1, func(j Job) bool { return j.State == "queued" })
			status := downloadStorage(t, s)
			if !status.Paused || status.ResumeAtBytes != 100+reserve || job.Failures != 2 || job.Error != "" || job.RetryAt != nil {
				t.Fatalf("interruption lost its recovery requirement or consumed retries: %+v, %+v", status, job)
			}
			if tc.checkFails && (status.Reason != "space_check_failed" || status.AvailableBytes != nil) {
				t.Fatalf("unknown space not reported: %+v", status)
			}
			if _, err := os.Stat(s.path(1) + ".part"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial survived interruption: %v", err)
			}
			s.Close()
			checkFails.Store(false)
			available.Store(99 + reserve)
			s = storageService(t, db, dir, transfer, space)
			if status := downloadStorage(t, s); !status.Paused || status.ResumeAtBytes != 100+reserve {
				t.Fatalf("cleanup or restart bypassed recovery: %+v", status)
			}
			available.Store(100 + reserve)
			s.signal()
			job = waitJob(t, s, 1, func(j Job) bool { return j.State == "completed" })
			if job.Failures != 2 || copies.Load() != 2 || job.SizeBytes != int64(len(archive)) {
				t.Fatalf("incorrect resumed transfer: %+v, copies=%d", job, copies.Load())
			}
		})
	}
}

func TestStorageRecoveryAccountsForPartialBeforeDeletingIt(t *testing.T) {
	db, dir := openDB(t, t.TempDir()), t.TempDir()
	if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at)
		VALUES (1, 'token', 'running', 1, 1);
		UPDATE panda_download_storage SET reason = 'low_space'`); err != nil {
		t.Fatal(err)
	}
	path := dir + "/[1].zip.part"
	if err := os.WriteFile(path, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := storageService(t, db, dir, nil, func(string) (int64, error) { return 106, nil })
	if status := downloadStorage(t, s); !status.Paused || status.ResumeAtBytes != 107 {
		t.Fatalf("recovery forgot unfinished cleanup: %+v", status)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery retained partial: %v", err)
	}
}

func TestCancellingOversizedArchiveReleasesQueue(t *testing.T) {
	db := openDB(t, t.TempDir())
	if _, err := db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at, expected_size_bytes)
		VALUES (1, 'token', 'queued', 1, 1, 1000), (2, 'token', 'queued', 2, 2, 0);
		UPDATE panda_download_storage SET reason = 'low_space', archive_bytes = 1000, gallery_id = 1`); err != nil {
		t.Fatal(err)
	}
	archive := zipBytes(t)
	s := storageService(t, db, t.TempDir(), transferFunc(func(_ context.Context, _ string, w io.Writer) (int64, error) {
		return io.Copy(w, bytes.NewReader(archive))
	}), func(string) (int64, error) { return 200, nil })
	if status := downloadStorage(t, s); !status.Paused || status.ResumeAtBytes != 1100 {
		t.Fatalf("missing archive reservation: %+v", status)
	}
	if job, err := s.Cancel(t.Context(), 1); err != nil || job.State != "cancelled" {
		t.Fatalf("cancel: %+v, %v", job, err)
	}
	waitJob(t, s, 2, func(j Job) bool { return j.State == "completed" })
	if downloadStorage(t, s).Paused {
		t.Fatal("cancelled archive still blocks queue")
	}
}
