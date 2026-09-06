package metadata

import (
	"context"
	"database/sql"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/metadata/dbgen"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func (s *store) failed(ctx context.Context, cause error, at time.Time) (time.Duration, error) {
	state, err := s.q.RetryState(ctx)
	if err != nil {
		return 0, err
	}
	// Persist an API-wide cooldown, so new discoveries and restarts cannot bypass it.
	delay := panda.RetryDelay(state.Failures, cause, at)
	err = s.q.RecordBatchFailure(ctx, dbgen.RecordBatchFailureParams{
		Failures: min(state.Failures+1, 7), NextAttemptAt: at.Add(delay).UnixMilli(),
		LastAttemptAt: sql.NullInt64{Int64: at.UnixMilli(), Valid: true},
		LastError:     sql.NullString{String: cause.Error(), Valid: true},
	})
	return delay, err
}
