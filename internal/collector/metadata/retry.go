package metadata

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
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
	delay := min(time.Minute<<min(state.Failures, 6), time.Hour)
	if httpErr, ok := errors.AsType[*panda.HTTPError](cause); ok {
		value := strings.TrimSpace(httpErr.RetryAfter)
		if seconds, err := strconv.ParseUint(value, 10, 32); err == nil {
			delay = max(delay, time.Duration(seconds)*time.Second)
		} else if deadline, err := http.ParseTime(value); err == nil {
			delay = max(delay, deadline.Sub(at))
		}
	}
	err = s.q.RecordBatchFailure(ctx, dbgen.RecordBatchFailureParams{
		Failures: min(state.Failures+1, 7), NextAttemptAt: at.Add(delay).UnixMilli(),
		LastAttemptAt: sql.NullInt64{Int64: at.UnixMilli(), Valid: true},
		LastError:     sql.NullString{String: cause.Error(), Valid: true},
	})
	return delay, err
}
