package gallery

import "context"

// Repository persists galleries and their source links. Independent galleries
// allow subsets, cross-library files and repeats; linked galleries allow reordering.
// Mutations are atomic; reading pages always returns consecutive numbers from 1.
type Repository interface {
	Create(ctx context.Context, title string, fileIDs []string) (Gallery, error)
	// CreateFromSource links a gallery to the source and selects supported images
	// in natural path order. ErrNoImages leaves no gallery behind.
	CreateFromSource(ctx context.Context, sourceID string) (Gallery, error)
	Get(ctx context.Context, id string) (Gallery, error)
	List(ctx context.Context) ([]Gallery, error)
	Pages(ctx context.Context, id string) ([]Page, error)
	Rename(ctx context.Context, id, title string) (Gallery, error)
	ReplacePages(ctx context.Context, id string, fileIDs []string) error
	// Delete is idempotent and leaves sources and their inventories intact.
	Delete(ctx context.Context, id string) error
}
