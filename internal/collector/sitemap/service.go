// Package sitemap collects gallery references through durable, manually started runs.
package sitemap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrState = errors.New("operation not allowed in current sitemap state")

const maxFailures = 3
const batchSize = 250

type activeRequest struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Service struct {
	db              *sql.DB
	q               *dbgen.Queries
	cfg             Config
	client          *http.Client
	logger          *slog.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	workers         sync.WaitGroup
	wake            chan struct{}
	mu              sync.Mutex
	active          *activeRequest
	requestInterval time.Duration
	retryDelay      func(int64, error, time.Time) time.Duration
}

func New(ctx context.Context, db *sql.DB, cfg Config, client *http.Client, logger *slog.Logger) (*Service, error) {
	if !validDocumentURL(cfg.URL) {
		return nil, fmt.Errorf("invalid sitemap index URL")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{db: db, q: dbgen.New(db), cfg: cfg, client: client, logger: logger,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), requestInterval: 10 * time.Second, retryDelay: panda.RetryDelay}
	if err := s.q.RecoverChildren(ctx); err != nil {
		cancel()
		return nil, err
	}
	run, err := s.q.GetRun(ctx)
	if err != nil {
		cancel()
		return nil, err
	}
	if run.NextRequestAt != 0 {
		// A redirect may have used the in-memory limiter after the persisted
		// deadline. Leave a full interval before requests after a restart.
		run.NextRequestAt = max(run.NextRequestAt, time.Now().Add(s.requestInterval).UnixMilli())
		if err := updateRun(ctx, s.q, run); err != nil {
			cancel()
			return nil, err
		}
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

func (s *Service) Start(ctx context.Context, force bool) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.q.GetRun(ctx)
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	if run.State == "running" {
		return s.status(ctx)
	}
	if s.active != nil {
		return collectorapi.SitemapStatus{}, ErrState
	}
	run = dbgen.SitemapRun{State: "running", IndexUrl: s.cfg.URL, StartedAt: time.Now().UnixMilli(), NextRequestAt: run.NextRequestAt}
	if force {
		run.Force = 1
	}
	err = s.transaction(ctx, func(q *dbgen.Queries) error {
		if err := q.ClearChildren(ctx); err != nil {
			return err
		}
		return updateRun(ctx, q, run)
	})
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	s.signal()
	return s.status(ctx)
}

func (s *Service) Retry(ctx context.Context) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	run, err := s.q.GetRun(ctx)
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	if (run.State != "incomplete" && run.State != "cancelled") || s.active != nil {
		return collectorapi.SitemapStatus{}, ErrState
	}
	run.State, run.FinishedAt, run.RetryAt, run.IndexFailures, run.LastError = "running", 0, 0, 0, ""
	err = s.transaction(ctx, func(q *dbgen.Queries) error {
		if err := q.RetryChildren(ctx); err != nil {
			return err
		}
		return updateRun(ctx, q, run)
	})
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	s.signal()
	return s.status(ctx)
}

func (s *Service) Cancel(ctx context.Context) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	run, err := s.q.GetRun(ctx)
	if err == nil && run.State != "running" && run.State != "cancelled" {
		err = ErrState
	}
	if err != nil {
		s.mu.Unlock()
		return collectorapi.SitemapStatus{}, err
	}
	run.State, run.FinishedAt, run.RetryAt = "cancelled", time.Now().UnixMilli(), 0
	if err := updateRun(ctx, s.q, run); err != nil {
		s.mu.Unlock()
		return collectorapi.SitemapStatus{}, err
	}
	active := s.active
	if active != nil {
		active.cancel()
	}
	s.mu.Unlock()
	if active != nil {
		select {
		case <-active.done:
		case <-ctx.Done():
			return collectorapi.SitemapStatus{}, ctx.Err()
		}
	}
	return s.Status(ctx)
}

func (s *Service) Status(ctx context.Context) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status(ctx)
}

func (s *Service) status(ctx context.Context) (collectorapi.SitemapStatus, error) {
	run, err := s.q.GetRun(ctx)
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	counts, err := s.q.ChildCounts(ctx)
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	status := collectorapi.SitemapStatus{State: run.State, Force: run.Force != 0, StartedAt: timestamp(run.StartedAt),
		FinishedAt: timestamp(run.FinishedAt), ChildrenTotal: counts.Total, ChildrenCompleted: counts.Completed,
		ChildrenSkipped: counts.Skipped, ChildrenFailed: counts.Failed, ReferencesFound: counts.Found,
		ReferencesImported: counts.Imported, InvalidLocations: counts.Invalid, LastError: run.LastError}
	if run.State == "running" {
		retryAt := run.RetryAt
		if run.IndexReady != 0 {
			child, err := s.q.NextChild(ctx)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return collectorapi.SitemapStatus{}, err
			}
			retryAt = child.RetryAt
		}
		retryAt = max(retryAt, run.NextRequestAt)
		if retryAt > time.Now().UnixMilli() {
			status.RetryAt = timestamp(retryAt)
		}
	}
	return status, nil
}

func timestamp(value int64) *time.Time {
	if value == 0 {
		return nil
	}
	at := time.UnixMilli(value)
	return &at
}

func (s *Service) run() {
	for s.ctx.Err() == nil {
		worked, err := s.step()
		if err != nil && s.ctx.Err() == nil {
			s.logger.Error("sitemap_storage_failed", "error", err)
			s.mu.Lock()
			recoverErr := s.q.RecoverChildren(s.ctx)
			s.mu.Unlock()
			if recoverErr != nil {
				s.logger.Error("sitemap_recovery_failed", "error", recoverErr)
			}
		}
		if !worked || err != nil {
			timer := time.NewTimer(time.Second)
			select {
			case <-s.ctx.Done():
			case <-s.wake:
			case <-timer.C:
			}
			timer.Stop()
		}
	}
}

func (s *Service) step() (bool, error) {
	s.mu.Lock()
	run, err := s.q.GetRun(s.ctx)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	if run.State != "running" {
		s.mu.Unlock()
		return false, nil
	}
	var child dbgen.SitemapChild
	if run.IndexReady != 0 {
		child, err = s.q.NextChild(s.ctx)
		if errors.Is(err, sql.ErrNoRows) {
			counts, err := s.q.ChildCounts(s.ctx)
			if err == nil {
				run.State, run.FinishedAt, run.RetryAt = "completed", time.Now().UnixMilli(), 0
				if counts.Failed > 0 {
					run.State, run.LastError = "incomplete", "child_sitemaps_failed"
				} else {
					run.LastError = ""
				}
				err = updateRun(s.ctx, s.q, run)
			}
			s.mu.Unlock()
			return true, err
		}
		if err != nil {
			s.mu.Unlock()
			return false, err
		}
	}
	due := max(run.NextRequestAt, run.RetryAt, child.RetryAt)
	if due > time.Now().UnixMilli() {
		s.mu.Unlock()
		return false, nil
	}
	ctx, cancel := context.WithCancel(s.ctx)
	active := &activeRequest{cancel: cancel, done: make(chan struct{})}
	run.NextRequestAt = time.Now().Add(s.requestInterval).UnixMilli()
	err = s.transaction(ctx, func(q *dbgen.Queries) error {
		if child.ID != 0 {
			child.State = "running"
			if err := updateChild(ctx, q, child); err != nil {
				return err
			}
			if err := q.ResetChildCounts(ctx, child.ID); err != nil {
				return err
			}
		}
		return updateRun(ctx, q, run)
	})
	if err != nil {
		cancel()
		s.mu.Unlock()
		return false, err
	}
	s.active = active
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.active = nil
		close(active.done)
		s.mu.Unlock()
	}()
	if child.ID == 0 {
		err = s.collectIndex(ctx, run)
	} else {
		err = s.collectChild(ctx, run, child)
	}
	return true, err
}

func (s *Service) request(ctx context.Context, address string, validator *dbgen.SitemapValidator) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	setValidators := func(req *http.Request) {
		// Redirects can lead to independent resources with identical validators.
		req.Header.Del("If-None-Match")
		req.Header.Del("If-Modified-Since")
		if validator == nil || req.URL.String() != validator.ResponseUrl {
			return
		}
		if validator.Etag != "" {
			req.Header.Set("If-None-Match", validator.Etag)
		}
		if validator.LastModified != "" {
			req.Header.Set("If-Modified-Since", validator.LastModified)
		}
	}
	setValidators(req)
	client := *s.client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if s.client.CheckRedirect != nil {
			if err := s.client.CheckRedirect(req, via); err != nil {
				return err
			}
		} else if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		setValidators(req)
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotModified {
		resp.Body.Close()
		return nil, &panda.HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	return resp, nil
}

func (s *Service) collectIndex(ctx context.Context, run dbgen.SitemapRun) error {
	resp, cause := s.request(ctx, run.IndexUrl, nil)
	var children []string
	if cause == nil {
		if resp.StatusCode != http.StatusOK {
			cause = panda.ErrInvalidSitemap
		} else {
			children, cause = panda.ParseSitemapIndex(resp.Body)
		}
		resp.Body.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	if cause != nil {
		s.failRun(&run, cause, true)
		return updateRun(ctx, s.q, run)
	}
	run.IndexReady, run.RetryAt, run.LastError = 1, 0, ""
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		for _, address := range children {
			if err := q.AddChild(ctx, address); err != nil {
				return err
			}
		}
		return updateRun(ctx, q, run)
	})
}

func (s *Service) collectChild(ctx context.Context, run dbgen.SitemapRun, child dbgen.SitemapChild) error {
	var validator *dbgen.SitemapValidator
	if run.Force == 0 {
		value, err := s.q.GetValidator(ctx, child.Url)
		if err == nil {
			validator = &value
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	resp, cause := s.request(ctx, child.Url, validator)
	skipped := false
	if cause == nil {
		if resp.StatusCode == http.StatusNotModified {
			if validator == nil || validator.ResponseUrl != resp.Request.URL.String() || (validator.Etag == "" && validator.LastModified == "") {
				cause = panda.ErrInvalidSitemap
			} else {
				skipped = true
			}
		} else {
			cause = s.importChild(ctx, child.ID, resp.Body)
		}
		resp.Body.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nil
	}
	if cause != nil {
		if storage, ok := errors.AsType[*storageError](cause); ok {
			return storage.err
		}
		child.State, child.LastError = "pending", failureCode(cause)
		if _, banned := errors.AsType[*panda.BanError](cause); !banned {
			child.Failures++
		}
		child.RetryAt = time.Now().Add(s.retryDelay(max(0, child.Failures-1), cause, time.Now())).UnixMilli()
		if child.Failures >= maxFailures {
			child.State, child.RetryAt = "failed", 0
		}
		s.failRun(&run, cause, false)
		return s.transaction(ctx, func(q *dbgen.Queries) error {
			if err := updateChild(ctx, q, child); err != nil {
				return err
			}
			return updateRun(ctx, q, run)
		})
	}
	child.State, child.RetryAt, child.LastError = "completed", 0, ""
	if skipped {
		child.State = "skipped"
	}
	return s.transaction(ctx, func(q *dbgen.Queries) error {
		if !skipped {
			if err := q.SaveValidator(ctx, dbgen.SaveValidatorParams{Url: child.Url, ResponseUrl: resp.Request.URL.String(), Etag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}); err != nil {
				return err
			}
		}
		return updateChild(ctx, q, child)
	})
}

func (s *Service) failRun(run *dbgen.SitemapRun, cause error, index bool) {
	at := time.Now()
	_, banned := errors.AsType[*panda.BanError](cause)
	run.LastError = failureCode(cause)
	if index {
		if !banned {
			run.IndexFailures++
		}
		run.RetryAt = at.Add(s.retryDelay(max(0, run.IndexFailures-1), cause, at)).UnixMilli()
		if run.IndexFailures >= maxFailures {
			run.State, run.FinishedAt, run.RetryAt = "incomplete", at.UnixMilli(), 0
		}
	}
	// A Retry-After or shared ban blocks subsequent children as well.
	httpErr, httpFailure := errors.AsType[*panda.HTTPError](cause)
	if banned || (httpFailure && httpErr.RetryAfter != "") {
		run.NextRequestAt = max(run.NextRequestAt, at.Add(s.retryDelay(0, cause, at)).UnixMilli())
	}
	s.logger.Warn("sitemap_attempt_failed", "reason", run.LastError, "index", index)
}

func failureCode(err error) string {
	if errors.Is(err, panda.ErrInvalidSitemap) {
		return "invalid_sitemap"
	}
	if _, ok := errors.AsType[*panda.BanError](err); ok {
		return "panda_banned"
	}
	if e, ok := errors.AsType[*panda.HTTPError](err); ok {
		return "upstream_http_" + strconv.Itoa(e.StatusCode)
	}
	return "request_failed"
}
