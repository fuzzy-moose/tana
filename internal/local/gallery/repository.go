package gallery

import "context"

// Repository persists galleries and their source links. Independent galleries
// allow subsets, cross-library files and repeats; linked galleries allow reordering.
// Mutations are atomic; reading pages always returns consecutive numbers from 1.
type Repository interface {
	Create(ctx context.Context, title string, fileIDs []int64) (Gallery, error)
	// CreateFromSource links a gallery to the source and selects supported images
	// in natural path order. ErrNoImages leaves no gallery behind.
	CreateFromSource(ctx context.Context, sourceID int64) (Gallery, error)
	Get(ctx context.Context, id int64) (Gallery, error)
	List(ctx context.Context) ([]Gallery, error)
	Pages(ctx context.Context, id int64) ([]Page, error)
	Rename(ctx context.Context, id int64, title string) (Gallery, error)
	ReplacePages(ctx context.Context, id int64, fileIDs []int64) error
	// Delete is idempotent and leaves sources and their inventories intact.
	Delete(ctx context.Context, id int64) error
}
