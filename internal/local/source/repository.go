package source

import "context"

// Repository catalogs sources without visiting the filesystem. Discovery must
// supply every contained file and verify that directory sources have no
// subdirectories. IDs survive reads; path-based matching does not infer moves.
type Repository interface {
	// Create atomically saves a source and its complete inventory.
	Create(ctx context.Context, libraryID int64, path string, kind Kind, files []string) (Source, error)
	Get(ctx context.Context, id int64) (Source, error)
	List(ctx context.Context, libraryID int64) ([]Source, error)
	Files(ctx context.Context, id int64) ([]File, error)
	// Delete is idempotent and also removes affected gallery pages.
	Delete(ctx context.Context, id int64) error
}
