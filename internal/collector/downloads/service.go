// Package downloads owns durable Panda download jobs and retained original ZIPs.
package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/downloads/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrInvalidReference = errors.New("invalid gallery reference")
var ErrState = errors.New("operation not allowed in current download state")
var ErrTokenConflict = errors.New("gallery token conflicts with existing download")
var ErrInvalidState = errors.New("invalid download state")

const maxFailures = 5

type ArchiveClient interface {
	GetArchiveURL(context.Context, panda.GalleryRef) (string, error)
}

type Job struct {
	GalleryID int64      `json:"gallery_id"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RetryAt   *time.Time `json:"retry_at,omitempty"`
	Failures  int64      `json:"failures"`
	SizeBytes int64      `json:"size_bytes"`
	Error     string     `json:"error,omitempty"`
}

type ListResult struct {
	Jobs   []Job            `json:"jobs"`
	Counts map[string]int64 `json:"counts"`
}

func publicJob(row dbgen.PandaDownload) Job {
	j := Job{GalleryID: row.GalleryID, State: row.State, CreatedAt: time.UnixMilli(row.CreatedAt),
		UpdatedAt: time.UnixMilli(row.UpdatedAt), Failures: row.Failures, SizeBytes: row.SizeBytes, Error: row.LastError}
	if row.RetryAt != 0 {
		at := time.UnixMilli(row.RetryAt)
		j.RetryAt = &at
	}
	return j
}

type activeJob struct {
	id     int64
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

type Service struct {
	db       *sql.DB
	q        *dbgen.Queries
	dir      string
	client   ArchiveClient
	transfer Transfer
	logger   *slog.Logger
	ctx      context.Context
	cancel   context.CancelFunc
	workers  sync.WaitGroup
	wake     chan struct{}
	// Serialize state changes and file publication with cancellation/deletion.
	mu     sync.Mutex
	active *activeJob
}

func New(ctx context.Context, db *sql.DB, dir string, client ArchiveClient, transfer Transfer, logger *slog.Logger) (*Service, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create download directory: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, q: dbgen.New(db), dir: abs, client: client, transfer: transfer, logger: logger,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1)}
	if err := s.recover(ctx); err != nil {
		cancel()
		return nil, err
	}
	s.workers.Go(s.run)
	return s, nil
}

func (s *Service) Close() { s.cancel(); s.workers.Wait() }

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) path(id int64) string {
	return filepath.Join(s.dir, fmt.Sprintf("[%d].zip", id))
}

func (s *Service) update(ctx context.Context, row dbgen.PandaDownload) error {
	return s.q.UpdateDownload(ctx, dbgen.UpdateDownloadParams{GalleryID: row.GalleryID, State: row.State,
		UpdatedAt: time.Now().UnixMilli(), RetryAt: row.RetryAt, Failures: row.Failures, SizeBytes: row.SizeBytes, LastError: row.LastError})
}

// EnqueueInTransaction admits a download atomically with a favorite discovery.
// Existing jobs, including cancelled and failed jobs, retain their state.
// The worker polls for committed work, so no wakeup is required.
func EnqueueInTransaction(ctx context.Context, tx *sql.Tx, ref panda.GalleryRef) error {
	return enqueue(ctx, dbgen.New(tx), ref)
}

func enqueue(ctx context.Context, q *dbgen.Queries, ref panda.GalleryRef) error {
	if ref.ID <= 0 || strings.TrimSpace(ref.Token) == "" || len(ref.Token) > 256 {
		return ErrInvalidReference
	}
	at := time.Now().UnixMilli()
	return q.EnqueueDownload(ctx, dbgen.EnqueueDownloadParams{GalleryID: ref.ID, Token: ref.Token, CreatedAt: at, UpdatedAt: at})
}

func (s *Service) Submit(ctx context.Context, ref panda.GalleryRef) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := enqueue(ctx, s.q, ref); err != nil {
		return Job{}, err
	}
	row, err := s.q.GetDownload(ctx, ref.ID)
	if err != nil {
		return Job{}, err
	}
	if row.Token != ref.Token {
		return Job{}, ErrTokenConflict
	}
	s.signal()
	return publicJob(row), nil
}

func (s *Service) Get(ctx context.Context, id int64) (Job, error) {
	row, err := s.q.GetDownload(ctx, id)
	return publicJob(row), err
}

func (s *Service) List(ctx context.Context, state string, limit, offset int64) (ListResult, error) {
	result := ListResult{Jobs: []Job{}, Counts: map[string]int64{
		"queued": 0, "running": 0, "completed": 0, "failed": 0, "cancelled": 0, "deleting": 0,
	}}
	if _, valid := result.Counts[state]; state != "" && !valid {
		return result, ErrInvalidState
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	counts, err := q.CountDownloadsByState(ctx)
	if err != nil {
		return result, err
	}
	for _, count := range counts {
		result.Counts[count.State] = count.Count
	}
	rows, err := q.ListDownloads(ctx, dbgen.ListDownloadsParams{State: state, PageLimit: limit, PageOffset: offset})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.Jobs = append(result.Jobs, publicJob(row))
	}
	return result, tx.Commit()
}

func (s *Service) Retry(ctx context.Context, id int64) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.q.GetDownload(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if (row.State != "failed" && row.State != "cancelled") || (s.active != nil && s.active.id == id) {
		return Job{}, ErrState
	}
	row.State, row.RetryAt, row.Failures, row.LastError = "queued", 0, 0, ""
	if err := s.update(ctx, row); err != nil {
		return Job{}, err
	}
	s.signal()
	return s.Get(ctx, id)
}

func (s *Service) Cancel(ctx context.Context, id int64) (Job, error) {
	s.mu.Lock()
	row, err := s.q.GetDownload(ctx, id)
	if err == nil && row.State != "queued" && row.State != "running" && row.State != "cancelled" {
		err = ErrState
	}
	if err != nil {
		s.mu.Unlock()
		return Job{}, err
	}
	row.State, row.RetryAt, row.LastError = "cancelled", 0, ""
	if err := s.update(ctx, row); err != nil {
		s.mu.Unlock()
		return Job{}, err
	}
	active := s.active
	if active != nil && active.id == id {
		active.cancel()
		s.mu.Unlock()
		select {
		case <-active.done:
			if active.err != nil {
				return Job{}, active.err
			}
		case <-ctx.Done():
			return Job{}, ctx.Err()
		}
	} else {
		err = removeFile(s.path(id) + ".part")
		s.mu.Unlock()
		if err != nil {
			return Job{}, err
		}
	}
	return s.Get(ctx, id)
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.q.GetDownload(ctx, id)
	if err != nil {
		return err
	}
	if (row.State != "completed" && row.State != "failed" && row.State != "cancelled" && row.State != "deleting") ||
		(s.active != nil && s.active.id == id) {
		return ErrState
	}
	// Persist intent first so a restart can finish interrupted deletion.
	row.State = "deleting"
	if err := s.update(ctx, row); err != nil {
		return err
	}
	if err := removeFile(s.path(id)); err != nil {
		return err
	}
	if err := removeFile(s.path(id) + ".part"); err != nil {
		return err
	}
	if err := syncDirectory(s.dir); err != nil {
		return err
	}
	return s.q.DeleteDownload(ctx, id)
}

// Open holds the state lock until it owns an open file, so concurrent deletion
// cannot replace the file between the state check and the open.
func (s *Service) Open(ctx context.Context, id int64) (*os.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := s.q.GetDownload(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.State != "completed" {
		return nil, ErrState
	}
	return os.Open(s.path(id))
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// The rename can reach disk before the completion record. Recover that archive
// instead of fetching it again; unfinished partial transfers restart from zero.
func (s *Service) recover(ctx context.Context) error {
	rows, err := s.q.RecoverDownloads(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		path := s.path(row.GalleryID)
		if err := removeFile(path + ".part"); err != nil {
			return err
		}
		switch row.State {
		case "deleting":
			if err := removeFile(path); err != nil {
				return err
			}
			if err := syncDirectory(s.dir); err != nil {
				return err
			}
			if err := s.q.DeleteDownload(ctx, row.GalleryID); err != nil {
				return err
			}
		case "running":
			row.State = "queued"
			if info, err := os.Stat(path); err == nil {
				if err := validateZIP(ctx, path); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if err := removeFile(path); err != nil {
						return err
					}
				} else {
					if err := syncDirectory(s.dir); err != nil {
						return err
					}
					row.State, row.SizeBytes = "completed", info.Size()
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := s.update(ctx, row); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) wait() {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
	case <-s.wake:
	case <-timer.C:
	}
}

func (s *Service) run() {
	for s.ctx.Err() == nil {
		s.mu.Lock()
		row, err := s.q.NextDownload(s.ctx, time.Now().UnixMilli())
		var ctx context.Context
		var active *activeJob
		if err == nil {
			row.State = "running"
			err = s.update(s.ctx, row)
			if err == nil {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(s.ctx)
				active = &activeJob{id: row.GalleryID, cancel: cancel, done: make(chan struct{})}
				s.active = active
			}
		}
		s.mu.Unlock()
		if err == nil {
			err = s.execute(ctx, row, active)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) && s.ctx.Err() == nil {
			s.logger.Error("download_storage_failed", "error", err)
			for s.ctx.Err() == nil {
				s.wait()
				s.mu.Lock()
				err = s.recover(s.ctx)
				s.mu.Unlock()
				if err == nil {
					break
				}
			}
		} else if err != nil {
			s.wait()
		}
	}
}

func (s *Service) execute(ctx context.Context, row dbgen.PandaDownload, active *activeJob) (err error) {
	defer func() {
		active.cancel()
		s.mu.Lock()
		s.active = nil
		active.err = err
		close(active.done)
		s.mu.Unlock()
	}()
	path := s.path(row.GalleryID)
	size, cause := s.fetch(ctx, row, path+".part")
	s.mu.Lock()
	defer s.mu.Unlock()
	// Cancel may have committed while the network request was in progress.
	if ctx.Err() != nil {
		return removeFile(path + ".part")
	}
	if cause == nil {
		cause = os.Rename(path+".part", path)
		if cause == nil {
			if err := syncDirectory(s.dir); err != nil {
				return err
			}
			row.State, row.SizeBytes, row.RetryAt, row.LastError = "completed", size, 0, ""
			return s.update(s.ctx, row)
		}
	}
	if err := removeFile(path + ".part"); err != nil {
		return err
	}
	row.State, row.LastError, row.RetryAt = "failed", failureCode(cause), 0
	if _, banned := errors.AsType[*panda.BanError](cause); banned {
		row.State = "queued"
	} else {
		row.Failures++
		if retryable(cause) && row.Failures < maxFailures {
			row.State = "queued"
		}
	}
	if row.State == "queued" {
		row.RetryAt = time.Now().Add(panda.RetryDelay(max(0, row.Failures-1), cause, time.Now())).UnixMilli()
	}
	s.logger.Warn("download_attempt_failed", "gallery_id", row.GalleryID, "reason", row.LastError, "state", row.State)
	return s.update(s.ctx, row)
}

func (s *Service) fetch(ctx context.Context, row dbgen.PandaDownload, path string) (int64, error) {
	address, err := s.client.GetArchiveURL(ctx, panda.GalleryRef{ID: row.GalleryID, Token: row.Token})
	if err != nil {
		return 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	size, err := s.transfer.Copy(ctx, address, file)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = validateZIP(ctx, path)
	}
	return size, err
}

func retryable(err error) bool {
	if errors.Is(err, panda.ErrArchivePage) {
		return false
	}
	if e, ok := errors.AsType[*panda.HTTPError](err); ok {
		return e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
	}
	if _, ok := errors.AsType[*os.PathError](err); ok {
		return false
	}
	return true
}

// Persist categories, never upstream URLs, tokens, response bodies, or cookies.
func failureCode(err error) string {
	switch {
	case errors.Is(err, panda.ErrArchivePage):
		return "archive_unavailable_or_unauthenticated"
	case errors.Is(err, errExpiredURL):
		return "archive_url_expired"
	case errors.Is(err, errInvalidZIP):
		return "invalid_zip"
	}
	if _, ok := errors.AsType[*panda.BanError](err); ok {
		return "panda_banned"
	}
	if e, ok := errors.AsType[*panda.HTTPError](err); ok {
		return "upstream_http_" + strconv.Itoa(e.StatusCode)
	}
	if _, ok := errors.AsType[*os.PathError](err); ok {
		return "storage_error"
	}
	return "transfer_failed"
}
