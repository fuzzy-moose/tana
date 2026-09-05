package library

import (
	"context"
	"time"
)

// Repository persists libraries and availability observations. Implementations
// must support concurrent calls and return ErrNotFound for missing Get/Rename
// targets. Delete is idempotent; updates never recreate deleted libraries.
type Repository interface {
	// Create atomically rejects overlapping roots with ErrRootConflict and
	// inserts an available library. name is nonempty and path is clean and absolute.
	Create(ctx context.Context, name, path string) (Library, error)
	List(ctx context.Context) ([]Library, error)
	Get(ctx context.Context, id string) (Library, error)
	Rename(ctx context.Context, id, name string) (Library, error)
	Delete(ctx context.Context, id string) error
	ResetAvailability(ctx context.Context) error
	UpdateAvailability(ctx context.Context, id, availability string, checkedAt time.Time) error
}
