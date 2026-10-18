// Package pandaban persists independent Panda request group cooldowns.
package pandaban

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collector/pandaban/dbgen"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

type State struct {
	q        *dbgen.Queries
	id       int64
	mu       sync.Mutex
	observed time.Time
}

// New returns the main unauthenticated request group's ban state.
func New(db *pgxpool.Pool) *State { return &State{q: dbgen.New(db), id: 1} }

// NewAuthenticated returns the ban shared by favorites and archive preparation.
func NewAuthenticated(db *pgxpool.Pool) *State { return &State{q: dbgen.New(db), id: 2} }

func (s *State) Until(ctx context.Context) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, err := s.q.BanUntil(ctx, s.id)
	return pgdb.Latest(at, &s.observed), err
}

// Reset lifts the group's ban, including one observed but not yet persisted.
func (s *State) Reset(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observed = time.Time{}
	return s.q.ResetBan(ctx, s.id)
}

func (s *State) Extend(ctx context.Context, until time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Keep the storage precision so remembered and persisted deadlines agree.
	until = until.Truncate(time.Microsecond)
	// Keep the process paused even if persisting an observed ban fails.
	if until.After(s.observed) {
		s.observed = until
	}
	return s.q.ExtendBan(ctx, dbgen.ExtendBanParams{ID: s.id, UntilAt: until})
}
