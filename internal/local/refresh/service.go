// Package refresh previews and removes confirmed-missing catalog sources.
package refresh

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/refresh/dbgen"
)

var ErrInvalidSelection = errors.New("refresh preview expired or selection is invalid; preview again")

type Gallery struct {
	ID           int64  `json:"id"`
	Title        string `json:"title"`
	Deleted      bool   `json:"deleted"`
	PagesRemoved int64  `json:"pages_removed"`
}

type Candidate struct {
	SourceID    int64     `json:"source_id"`
	LibraryID   int64     `json:"library_id"`
	LibraryName string    `json:"library_name"`
	Path        string    `json:"path"`
	Galleries   []Gallery `json:"galleries"`
}

type Skipped struct {
	LibraryID   int64  `json:"library_id"`
	LibraryName string `json:"library_name"`
	SourceID    int64  `json:"source_id,omitempty"`
	Path        string `json:"path"`
	Reason      string `json:"reason"`
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
	Removed []int64   `json:"removed"`
	Failed  []Failure `json:"failed"`
}

type reviewed struct {
	row       dbgen.ListSourcesRow
	galleries []dbgen.ListAffectedGalleriesRow
}

type plan struct {
	created    time.Time
	candidates map[int64]reviewed
}

type Service struct {
	db           *sql.DB
	dirFS        func(string) fs.FS
	probeTimeout time.Duration
	probeSlots   chan struct{}
	mu           sync.Mutex
	plans        map[string]plan
}

func New(db *sql.DB, dirFS func(string) fs.FS) *Service {
	return &Service{db: db, dirFS: dirFS, probeTimeout: 30 * time.Second, probeSlots: make(chan struct{}, 4), plans: map[string]plan{}}
}

// Preview checks one library, or all libraries when libraryID is zero.
func (s *Service) Preview(ctx context.Context, libraryID int64) (Preview, error) {
	result := Preview{Candidates: []Candidate{}, Skipped: []Skipped{}}
	if libraryID != 0 {
		if _, err := library.NewSQLiteRepository(s.db).Get(ctx, libraryID); err != nil {
			return result, err
		}
	}
	q := dbgen.New(s.db)
	rows, err := q.ListSources(ctx, libraryID)
	if err != nil {
		return result, err
	}
	p := plan{created: time.Now(), candidates: map[int64]reviewed{}}
	for len(rows) > 0 {
		end := 1
		for end < len(rows) && rows[end].LibraryID == rows[0].LibraryID {
			end++
		}
		group := rows[:end]
		rows = rows[end:]
		observed := s.probe(ctx, group)
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if observed.blocked != "" {
			row := group[0]
			result.Skipped = append(result.Skipped, Skipped{LibraryID: row.LibraryID, LibraryName: row.LibraryName, Path: row.LibraryPath, Reason: observed.blocked})
			continue
		}
		for _, row := range group {
			fullPath := filepath.Join(row.LibraryPath, filepath.FromSlash(row.Path))
			if reason := observed.uncertain[row.ID]; reason != "" {
				result.Skipped = append(result.Skipped, Skipped{LibraryID: row.LibraryID, LibraryName: row.LibraryName, SourceID: row.ID, Path: fullPath, Reason: reason})
				continue
			}
			if !observed.missing[row.ID] {
				continue
			}
			galleries, err := q.ListAffectedGalleries(ctx, row.ID)
			if err != nil {
				return result, err
			}
			candidate := Candidate{SourceID: row.ID, LibraryID: row.LibraryID, LibraryName: row.LibraryName, Path: fullPath, Galleries: []Gallery{}}
			for _, g := range galleries {
				candidate.Galleries = append(candidate.Galleries, Gallery{ID: g.ID, Title: g.Title, Deleted: g.Deleted != 0, PagesRemoved: g.PagesRemoved})
			}
			result.Candidates = append(result.Candidates, candidate)
			p.candidates[row.ID] = reviewed{row: row, galleries: galleries}
		}
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
	result := Result{Removed: []int64{}, Failed: []Failure{}}
	s.mu.Lock()
	p, ok := s.plans[planID]
	selected := map[int64]bool{}
	valid := ok && time.Since(p.created) <= 30*time.Minute && len(sourceIDs) > 0
	for _, id := range sourceIDs {
		_, exists := p.candidates[id]
		valid = valid && exists && !selected[id]
		selected[id] = true
	}
	if !valid {
		s.mu.Unlock()
		return result, ErrInvalidSelection
	}
	delete(s.plans, planID)
	s.mu.Unlock()

	groups := map[int64][]int64{}
	var libraryIDs []int64
	for _, id := range sourceIDs {
		libraryID := p.candidates[id].row.LibraryID
		if len(groups[libraryID]) == 0 {
			libraryIDs = append(libraryIDs, libraryID)
		}
		groups[libraryID] = append(groups[libraryID], id)
	}
	for _, libraryID := range libraryIDs {
		removed, failed, err := s.remove(ctx, libraryID, groups[libraryID], p)
		if err != nil {
			return result, err
		}
		result.Removed = append(result.Removed, removed...)
		result.Failed = append(result.Failed, failed...)
	}
	return result, nil
}

func (s *Service) remove(ctx context.Context, libraryID int64, ids []int64, p plan) ([]int64, []Failure, error) {
	rows, err := dbgen.New(s.db).ListSources(ctx, libraryID)
	if err != nil {
		return nil, nil, err
	}
	observed := s.probe(ctx, rows)
	// Filesystem work stays outside the transaction. Revalidate the catalog
	// under its write lock so concurrent scans or edits cannot change the impact.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	q := dbgen.New(tx)
	current, err := q.ListSources(ctx, libraryID)
	if err != nil {
		return nil, nil, err
	}
	if !slices.Equal(rows, current) {
		observed.blocked = "Library catalog changed during the storage check; preview again."
	}
	byID := make(map[int64]dbgen.ListSourcesRow, len(current))
	for _, row := range current {
		byID[row.ID] = row
	}
	var removed []int64
	var failed []Failure
	for _, id := range ids {
		before := p.candidates[id]
		reason := observed.blocked
		if reason == "" && byID[id] != before.row {
			reason = "Source or library changed since preview."
		}
		if reason == "" {
			reason = observed.uncertain[id]
		}
		if reason == "" && !observed.missing[id] {
			reason = "Source is present again; catalog entry was kept."
		}
		if reason == "" {
			galleries, err := q.ListAffectedGalleries(ctx, id)
			if err != nil {
				return nil, nil, err
			}
			if !slices.Equal(galleries, before.galleries) {
				reason = "Affected galleries changed since preview; preview again."
			}
		}
		if reason != "" {
			failed = append(failed, Failure{SourceID: id, Reason: reason})
			continue
		}
		if err := q.DeleteSource(ctx, id); err != nil {
			return nil, nil, err
		}
		removed = append(removed, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return removed, failed, nil
}
