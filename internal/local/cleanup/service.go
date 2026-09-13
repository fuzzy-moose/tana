// Package cleanup previews and permanently removes superseded local archives.
package cleanup

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/cleanup/dbgen"
)

var (
	ErrInvalidSelection = errors.New("cleanup review expired or selection is invalid; preview again")
	ErrUnavailable      = errors.New("collected Panda metadata is unavailable")
)

type lookupClient interface {
	Lookup(context.Context, []int64) (collectorapi.LookupResult, error)
}

type Source struct {
	ID          int64  `json:"id"`
	LibraryID   int64  `json:"library_id"`
	LibraryName string `json:"library_name"`
	Path        string `json:"path"`
	PandaID     int64  `json:"panda_id"`
	Title       string `json:"title"`
}

type Candidate struct {
	Source      Source `json:"source"`
	Replacement Source `json:"replacement"`
	SizeBytes   int64  `json:"size_bytes"`
}

type Skipped struct {
	SourceID int64  `json:"source_id"`
	Path     string `json:"path"`
	Reason   string `json:"reason"`
}

type Preview struct {
	PlanID     string      `json:"plan_id"`
	Candidates []Candidate `json:"candidates"`
	Skipped    []Skipped   `json:"skipped"`
}

type Failure struct {
	SourceID int64  `json:"source_id"`
	Reason   string `json:"reason"`
}

type Result struct {
	Deleted []int64   `json:"deleted"`
	Failed  []Failure `json:"failed"`
}

type reviewed struct{ source, replacement snapshot }
type plan struct {
	created    time.Time
	candidates map[int64]reviewed
}

type Service struct {
	db     *sql.DB
	client lookupClient
	mu     sync.Mutex
	plans  map[string]plan

	canonicalPath func(dbgen.ListSourcesRow) (string, error)
	probeTimeout  time.Duration
	probeSlots    chan struct{}
}

func New(db *sql.DB, client lookupClient) *Service {
	return &Service{
		db: db, client: client, plans: map[string]plan{},
		canonicalPath: filepathCanonical, probeTimeout: 5 * time.Second,
		probeSlots: make(chan struct{}, 4),
	}
}

func (s *Service) Preview(ctx context.Context) (Preview, error) {
	result := Preview{Candidates: []Candidate{}, Skipped: []Skipped{}}
	if s.client == nil {
		return result, ErrUnavailable
	}
	rows, err := dbgen.New(s.db).ListSources(ctx)
	if err != nil {
		return result, err
	}
	items := make([]snapshot, len(rows))
	counts, paths := map[int64]int{}, map[string]int{}
	ids := make([]int64, 0, len(rows))
	for i, row := range rows {
		items[i] = inspect(row)
		id := items[i].public.PandaID
		if id != 0 {
			counts[id]++
			ids = append(ids, id)
		}
		if items[i].canonical != "" {
			paths[items[i].canonical]++
		}
	}
	values, err := s.metadata(ctx, ids)
	if err != nil {
		return result, err
	}
	available := map[int64]int{}
	chainReasons := map[int64]string{}
	for i := range items {
		item := &items[i]
		id := item.public.PandaID
		item.public.Title = title(values[id])
		switch {
		case item.reason != "":
		case id == 0:
			item.reason = "No Panda ID in the source name"
		case counts[id] > 1:
			item.reason = "Panda ID matches multiple local sources"
		case paths[item.canonical] > 1:
			item.reason = "Multiple catalog sources share this filesystem path"
		default:
			_, chainReasons[id] = ancestry(id, values)
			if chainReasons[id] == "Parent chain contains a cycle" || chainReasons[id] == "Parent reference conflicts with collected metadata" {
				item.reason = chainReasons[id]
			} else {
				available[id] = i
			}
		}
	}
	// Retain terminal local versions. Every preselected candidate therefore has
	// the same visible replacement regardless of exclusions or execution order.
	descendants := map[int64][]int{}
	for i, item := range items {
		if item.reason != "" {
			continue
		}
		ancestors, _ := ancestry(item.public.PandaID, values)
		for _, id := range ancestors {
			if _, ok := available[id]; ok {
				descendants[id] = append(descendants[id], i)
			}
		}
	}
	p := plan{created: time.Now(), candidates: map[int64]reviewed{}}
	for _, item := range items {
		reason := item.reason
		if reason == "" && item.row.Kind != "archive" {
			reason = "Directory sources are excluded"
		}
		if reason == "" && item.row.Protected != 0 {
			reason = "Source is used by an independent gallery"
		}
		var replacement *snapshot
		if reason == "" {
			for _, i := range descendants[item.public.PandaID] {
				other := &items[i]
				if len(descendants[other.public.PandaID]) == 0 && !os.SameFile(item.file, other.file) {
					replacement = other
					break
				}
			}
			if replacement == nil {
				reason = chainReasons[item.public.PandaID]
				if reason == "" {
					reason = "No proven newer local source is present"
				}
			}
		}
		if reason != "" {
			result.Skipped = append(result.Skipped, Skipped{SourceID: item.row.ID, Path: item.public.Path, Reason: reason})
			continue
		}
		result.Candidates = append(result.Candidates, Candidate{Source: item.public, Replacement: replacement.public, SizeBytes: item.file.Size()})
		p.candidates[item.row.ID] = reviewed{source: item, replacement: *replacement}
	}
	var token [24]byte
	if _, err := rand.Read(token[:]); err != nil {
		return result, err
	}
	result.PlanID = hex.EncodeToString(token[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, previous := range s.plans {
		if time.Since(previous.created) > 30*time.Minute {
			delete(s.plans, id)
		}
	}
	if len(s.plans) >= 8 {
		var oldestID string
		var oldest time.Time
		for id, previous := range s.plans {
			if oldestID == "" || previous.created.Before(oldest) {
				oldestID, oldest = id, previous.created
			}
		}
		delete(s.plans, oldestID)
	}
	s.plans[result.PlanID] = p
	return result, nil
}

func (s *Service) Execute(ctx context.Context, planID string, sourceIDs []int64) (Result, error) {
	result := Result{Deleted: []int64{}, Failed: []Failure{}}
	s.mu.Lock()
	p, ok := s.plans[planID]
	selected := map[int64]bool{}
	valid := ok && time.Since(p.created) <= 30*time.Minute && len(sourceIDs) != 0
	for _, id := range sourceIDs {
		_, exists := p.candidates[id]
		valid = valid && exists && !selected[id]
		selected[id] = true
	}
	for _, id := range sourceIDs {
		valid = valid && !selected[p.candidates[id].replacement.row.ID]
	}
	if !valid {
		s.mu.Unlock()
		return result, ErrInvalidSelection
	}
	delete(s.plans, planID)
	s.mu.Unlock()
	rows, err := dbgen.New(s.db).ListSources(ctx)
	if err != nil {
		return result, err
	}
	catalog, err := s.probeCatalog(ctx, rows)
	if err != nil {
		return result, err
	}
	// Hold one immediate transaction so concurrent scans and gallery edits
	// cannot change the catalog between revalidation and filesystem deletion.
	// Cancellation stops further deletions, but must commit completed ones.
	commitCtx := context.WithoutCancel(ctx)
	tx, err := s.db.BeginTx(commitCtx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	rows, err = q.ListSources(ctx)
	if err != nil {
		return result, err
	}
	if !catalog.revalidate(rows) {
		return result, ErrInvalidSelection
	}
	for _, id := range sourceIDs {
		if err := deleteArchive(ctx, tx, catalog, p.candidates[id]); err != nil {
			result.Failed = append(result.Failed, Failure{SourceID: id, Reason: err.Error()})
		} else {
			result.Deleted = append(result.Deleted, id)
		}
	}
	if err := tx.Commit(); err != nil {
		for _, id := range result.Deleted {
			result.Failed = append(result.Failed, Failure{SourceID: id, Reason: fmt.Sprintf("Archive deleted, but catalog update failed: %v", err)})
		}
		result.Deleted = []int64{}
	}
	return result, nil
}

type catalogSnapshot struct {
	rows  map[int64]dbgen.ListSourcesRow
	ids   map[int64]int
	paths map[string]int
}

func (s *Service) probeCatalog(ctx context.Context, rows []dbgen.ListSourcesRow) (catalogSnapshot, error) {
	catalog := catalogSnapshot{rows: map[int64]dbgen.ListSourcesRow{}, ids: map[int64]int{}, paths: map[string]int{}}
	for _, row := range rows {
		catalog.rows[row.ID] = row
		catalog.ids[sourcePandaID(row)]++
		canonical, err := s.resolveCanonical(ctx, row)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// An unfinished probe cannot rule out an alias of a selected archive.
			return catalogSnapshot{}, err
		}
		if err == nil {
			catalog.paths[canonical]++
		}
	}
	return catalog, nil
}

func (c catalogSnapshot) revalidate(rows []dbgen.ListSourcesRow) bool {
	if len(rows) != len(c.rows) {
		return false
	}
	for _, row := range rows {
		if !sameAssociation(c.rows[row.ID], row) {
			return false
		}
		// Gallery references may have changed without changing a source's path.
		c.rows[row.ID] = row
	}
	return true
}

func deleteArchive(ctx context.Context, tx *sql.Tx, catalog catalogSnapshot, entry reviewed) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if catalog.ids[entry.source.public.PandaID] != 1 || catalog.ids[entry.replacement.public.PandaID] != 1 ||
		catalog.paths[entry.source.canonical] != 1 || catalog.paths[entry.replacement.canonical] != 1 {
		return errors.New("Source association became ambiguous; preview again")
	}
	old := inspect(catalog.rows[entry.source.row.ID])
	replacement := inspect(catalog.rows[entry.replacement.row.ID])
	if old.row.ID == 0 || !unchanged(entry.source, old) || old.file.Size() != entry.source.file.Size() || !old.file.ModTime().Equal(entry.source.file.ModTime()) {
		return errors.New("Archive changed or became unavailable since review")
	}
	if old.row.Protected != 0 {
		return errors.New("Source is now used by an independent gallery")
	}
	if replacement.row.ID == 0 || !unchanged(entry.replacement, replacement) || os.SameFile(old.file, replacement.file) {
		return errors.New("Reviewed replacement changed or is no longer present")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	commitCtx := context.WithoutCancel(ctx)
	if _, err := tx.ExecContext(commitCtx, "SAVEPOINT cleanup_source"); err != nil {
		return err
	}
	defer tx.ExecContext(commitCtx, "RELEASE SAVEPOINT cleanup_source")
	if err := dbgen.New(tx).DeleteSource(commitCtx, old.row.ID); err != nil {
		_, _ = tx.ExecContext(commitCtx, "ROLLBACK TO SAVEPOINT cleanup_source")
		return err
	}
	if err := removeArchive(old); err != nil {
		_, _ = tx.ExecContext(commitCtx, "ROLLBACK TO SAVEPOINT cleanup_source")
		return fmt.Errorf("Could not delete archive: %w", err)
	}
	return nil
}
