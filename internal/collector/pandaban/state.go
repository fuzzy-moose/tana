// Package pandaban persists independent Panda request group cooldowns.
package pandaban

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban/dbgen"
)

type State struct {
	q        *dbgen.Queries
	id       int64
	mu       sync.Mutex
	observed time.Time
}

// New returns the main unauthenticated request group's ban state.
func New(db *sql.DB) *State { return &State{q: dbgen.New(db), id: 1} }

// NewAuthenticated returns the ban shared by favorites and archive preparation.
func NewAuthenticated(db *sql.DB) *State { return &State{q: dbgen.New(db), id: 2} }

func (s *State) Until(ctx context.Context) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, err := s.q.BanUntil(ctx, s.id)
	until := time.UnixMilli(at)
	if s.observed.After(until) {
		until = s.observed
	}
	return until, err
}

func (s *State) Extend(ctx context.Context, until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the process paused even if persisting an observed ban fails.
	if until.After(s.observed) {
		s.observed = until
	}
	return s.q.ExtendBan(ctx, dbgen.ExtendBanParams{ID: s.id, UntilAt: until.UnixMilli()})
}
