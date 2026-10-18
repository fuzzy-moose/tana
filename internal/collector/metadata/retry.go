package metadata

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func (s *store) failed(ctx context.Context, cause error, at time.Time) (time.Duration, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := s.q.WithTx(tx)
	state, err := q.LockRetryState(ctx)
	if err != nil {
		return 0, err
	}
	// Persist an API-wide cooldown, so new discoveries and restarts cannot bypass it.
	delay := panda.RetryDelay(state.Failures, cause, at)
	if err := q.RecordBatchFailure(ctx, dbgen.RecordBatchFailureParams{
		Failures: min(state.Failures+1, 7), NextAttemptAt: new(at.Add(delay)),
		LastAttemptAt: &at,
		LastError:     pgtype.Text{String: cause.Error(), Valid: true},
	}); err != nil {
		return 0, err
	}
	return delay, tx.Commit(ctx)
}
