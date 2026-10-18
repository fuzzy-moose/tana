package pgdb

import (
	"time"

	"github.com/jackc/pgx/v5"
)

// ReadOnly begins a read-only transaction at the default isolation level.
var ReadOnly = pgx.TxOptions{AccessMode: pgx.ReadOnly}

// NullTime returns nil for the zero time, the unset value of a nullable
// TIMESTAMPTZ column, and a pointer to t otherwise.
func NullTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// Latest returns the latest of times; nil and zero times count as unset.
func Latest(times ...*time.Time) time.Time {
	var at time.Time
	for _, t := range times {
		if t != nil && t.After(at) {
			at = *t
		}
	}
	return at
}

// UTC returns t in UTC, or nil; pgx decodes TIMESTAMPTZ in time.Local.
func UTC(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	at := t.UTC()
	return &at
}
