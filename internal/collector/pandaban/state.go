// Package pandaban persists the cooldown shared by metadata and favorites.
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
	mu       sync.Mutex
	observed time.Time
}

func New(db *sql.DB) *State { return &State{q: dbgen.New(db)} }

func (s *State) Until(ctx context.Context) (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	at, err := s.q.BanUntil(ctx)
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
	return s.q.ExtendBan(ctx, until.UnixMilli())
}
