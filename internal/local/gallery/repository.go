package gallery

import "context"

// Repository persists galleries independently of libraries. Page lists are
// ordered source-file IDs, allowing subsets, cross-library files and repeats.
// Mutations are atomic; reading pages always returns consecutive numbers from 1.
type Repository interface {
	Create(ctx context.Context, title string, fileIDs []string) (Gallery, error)
	// CreateFromSource selects supported images in natural path order, deriving
	// a title from the source name. ErrNoImages leaves no gallery behind.
	CreateFromSource(ctx context.Context, sourceID string) (Gallery, error)
	Get(ctx context.Context, id string) (Gallery, error)
	List(ctx context.Context) ([]Gallery, error)
	Pages(ctx context.Context, id string) ([]Page, error)
	Rename(ctx context.Context, id, title string) (Gallery, error)
	ReplacePages(ctx context.Context, id string, fileIDs []string) error
	// Delete is idempotent and leaves sources and their inventories intact.
	Delete(ctx context.Context, id string) error
}
