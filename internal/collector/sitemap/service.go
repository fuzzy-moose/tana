// Package sitemap collects gallery references through durable, manually started runs.
package sitemap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

var ErrState = errors.New("operation not allowed in current sitemap state")

const maxFailures = 3
const batchSize = 250

type activeRequest struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type Service struct {
	db              *pgxpool.Pool
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

func New(ctx context.Context, db *pgxpool.Pool, cfg Config, client *http.Client, logger *slog.Logger) (*Service, error) {
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
	// A redirect may have used the in-memory limiter after the persisted
	// deadline. Leave a full interval before requests after a restart.
	if err := s.q.DelayNextRequest(ctx, time.Now().Add(s.requestInterval)); err != nil {
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

// ClearBanRetries resumes requests waiting only for the main ban while
// keeping one request interval after the response that reported it.
func (s *Service) ClearBanRetries(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.transaction(ctx, func(q *dbgen.Queries) error {
		if err := q.ClearRunBanRetry(ctx, time.Now().Add(s.requestInterval)); err != nil {
			return err
		}
		return q.ClearChildBanRetries(ctx)
	})
	if err == nil {
		s.signal()
	}
	return err
}

func (s *Service) Start(ctx context.Context, force bool) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	started := false
	err := s.withRun(ctx, func(q *dbgen.Queries, run *dbgen.SitemapRun) error {
		if run.State == "running" {
			return nil
		}
		if s.active != nil {
			return ErrState
		}
		if err := q.ClearChildren(ctx); err != nil {
			return err
		}
		started = true
		return updateRun(ctx, q, dbgen.SitemapRun{State: "running", IndexUrl: s.cfg.URL, StartedAt: new(time.Now()),
			NextRequestAt: run.NextRequestAt, Force: force})
	})
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	if started {
		s.signal()
	}
	return s.status(ctx)
}

func (s *Service) Retry(ctx context.Context) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.withRun(ctx, func(q *dbgen.Queries, run *dbgen.SitemapRun) error {
		if (run.State != "incomplete" && run.State != "cancelled") || s.active != nil {
			return ErrState
		}
		run.State, run.FinishedAt, run.RetryAt, run.IndexFailures, run.LastError = "running", nil, nil, 0, ""
		if err := q.RetryChildren(ctx); err != nil {
			return err
		}
		return updateRun(ctx, q, *run)
	})
	if err != nil {
		return collectorapi.SitemapStatus{}, err
	}
	s.signal()
	return s.status(ctx)
}

func (s *Service) Cancel(ctx context.Context) (collectorapi.SitemapStatus, error) {
	s.mu.Lock()
	err := s.withRun(ctx, func(q *dbgen.Queries, run *dbgen.SitemapRun) error {
		if run.State != "running" && run.State != "cancelled" {
			return ErrState
		}
		run.State, run.FinishedAt, run.RetryAt = "cancelled", new(time.Now()), nil
		return updateRun(ctx, q, *run)
	})
	if err != nil {
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
	status := collectorapi.SitemapStatus{State: run.State, Force: run.Force, StartedAt: run.StartedAt,
		FinishedAt: run.FinishedAt, ChildrenTotal: counts.Total, ChildrenCompleted: counts.Completed,
		ChildrenSkipped: counts.Skipped, ChildrenFailed: counts.Failed, ReferencesFound: counts.Found,
		ReferencesImported: counts.Imported, InvalidLocations: counts.Invalid, LastError: run.LastError}
	if run.State == "running" {
		retryAt := run.RetryAt
		if run.IndexReady {
			child, err := s.q.NextChild(ctx)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return collectorapi.SitemapStatus{}, err
			}
			retryAt = child.RetryAt
		}
		if at := pgdb.Latest(retryAt, run.NextRequestAt); at.After(time.Now()) {
			status.RetryAt = &at
		}
	}
	return status, nil
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
	var run dbgen.SitemapRun
	var child dbgen.SitemapChild
	worked, claimed := false, false
	err := s.whileRunning(s.ctx, func(q *dbgen.Queries, locked *dbgen.SitemapRun) error {
		run = *locked
		if run.IndexReady {
			var err error
			child, err = q.ClaimChild(s.ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				// Pending children locked by another writer are not finished.
				if pending, err := q.HasPendingChild(s.ctx); err != nil || pending {
					return err
				}
				counts, err := q.ChildCounts(s.ctx)
				if err != nil {
					return err
				}
				run.State, run.FinishedAt, run.RetryAt = "completed", new(time.Now()), nil
				if counts.Failed > 0 {
					run.State, run.LastError = "incomplete", "child_sitemaps_failed"
				} else {
					run.LastError = ""
				}
				worked = true
				return updateRun(s.ctx, q, run)
			}
			if err != nil {
				return err
			}
		}
		due := pgdb.Latest(run.NextRequestAt, run.RetryAt, child.RetryAt)
		if due.After(time.Now()) {
			return nil
		}
		if child.ID != 0 {
			started, err := q.StartChild(s.ctx, child.ID)
			if err != nil || started == 0 {
				return err
			}
			child.State = "running"
		}
		run.NextRequestAt = new(time.Now().Add(s.requestInterval))
		claimed = true
		return updateRun(s.ctx, q, run)
	})
	if err != nil || !claimed {
		s.mu.Unlock()
		return worked, err
	}
	ctx, cancel := context.WithCancel(s.ctx)
	active := &activeRequest{cancel: cancel, done: make(chan struct{})}
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
	return s.whileRunning(ctx, func(q *dbgen.Queries, run *dbgen.SitemapRun) error {
		if cause != nil {
			s.failRun(run, cause, true)
			return updateRun(ctx, q, *run)
		}
		run.IndexReady, run.RetryAt, run.LastError = true, nil, ""
		for _, address := range children {
			if err := q.AddChild(ctx, address); err != nil {
				return err
			}
		}
		return updateRun(ctx, q, *run)
	})
}

func (s *Service) collectChild(ctx context.Context, run dbgen.SitemapRun, child dbgen.SitemapChild) error {
	var validator *dbgen.SitemapValidator
	if !run.Force {
		value, err := s.q.GetValidator(ctx, child.Url)
		if err == nil {
			validator = &value
		} else if !errors.Is(err, pgx.ErrNoRows) {
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
		child.RetryAt = new(time.Now().Add(s.retryDelay(max(0, child.Failures-1), cause, time.Now())))
		if child.Failures >= maxFailures {
			child.State, child.RetryAt = "failed", nil
		}
		return s.whileRunning(ctx, func(q *dbgen.Queries, run *dbgen.SitemapRun) error {
			s.failRun(run, cause, false)
			if err := updateChild(ctx, q, child); err != nil {
				return err
			}
			return updateRun(ctx, q, *run)
		})
	}
	child.State, child.RetryAt, child.LastError = "completed", nil, ""
	if skipped {
		child.State = "skipped"
	}
	return s.whileRunning(ctx, func(q *dbgen.Queries, _ *dbgen.SitemapRun) error {
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
		run.RetryAt = new(at.Add(s.retryDelay(max(0, run.IndexFailures-1), cause, at)))
		if run.IndexFailures >= maxFailures {
			run.State, run.FinishedAt, run.RetryAt = "incomplete", &at, nil
		}
	}
	// A Retry-After or shared ban blocks subsequent children as well.
	httpErr, httpFailure := errors.AsType[*panda.HTTPError](cause)
	if banned || (httpFailure && httpErr.RetryAfter != "") {
		next := at.Add(s.retryDelay(0, cause, at))
		run.NextRequestAt = new(pgdb.Latest(run.NextRequestAt, &next))
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
