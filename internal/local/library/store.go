package library

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/local/library/dbgen"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// Store persists libraries using an application-owned database.
type Store struct {
	db      *pgxpool.Pool
	queries *dbgen.Queries
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, queries: dbgen.New(db)}
}

func (r *Store) Create(ctx context.Context, name, root string) (Library, error) {
	// The table lock serializes validation and insertion, including
	// registrations through other connections to the same database.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Library{}, err
	}
	defer tx.Rollback(ctx)
	q := r.queries.WithTx(tx)
	if err := q.LockLibraries(ctx); err != nil {
		return Library{}, err
	}
	roots, err := q.ListLibraries(ctx)
	if err != nil {
		return Library{}, err
	}
	for _, existing := range roots {
		if containsPath(root, existing.Path) || containsPath(existing.Path, root) {
			return Library{}, ErrRootConflict
		}
	}
	row, err := q.CreateLibrary(ctx, dbgen.CreateLibraryParams{
		Name: name, Path: root,
		LastCheckedAt: new(time.Now()),
	})
	if err != nil {
		return Library{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Library{}, err
	}
	return fromRow(row), nil
}

func (r *Store) List(ctx context.Context) ([]Library, error) {
	rows, err := r.queries.ListLibraries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Library, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRow(row))
	}
	return result, nil
}

func (r *Store) Get(ctx context.Context, id int64) (Library, error) {
	row, err := r.queries.GetLibrary(ctx, id)
	return fromRow(row), domainError(err)
}

func (r *Store) Rename(ctx context.Context, id int64, name string) (Library, error) {
	row, err := r.queries.RenameLibrary(ctx, dbgen.RenameLibraryParams{ID: id, Name: name})
	return fromRow(row), domainError(err)
}

func (r *Store) Delete(ctx context.Context, id int64) error {
	// The database owns library records; this operation never visits the root.
	return r.queries.DeleteLibrary(ctx, id)
}

func (r *Store) ResetAvailability(ctx context.Context) error {
	return r.queries.ResetAvailability(ctx)
}

func (r *Store) UpdateAvailability(ctx context.Context, id int64, availability string, checkedAt time.Time) error {
	return r.queries.UpdateAvailability(ctx, dbgen.UpdateAvailabilityParams{
		ID: id, Availability: availability,
		LastCheckedAt: &checkedAt,
	})
}

func fromRow(row dbgen.Library) Library {
	library := Library{ID: row.ID, Name: row.Name, Path: row.Path, Availability: row.Availability}
	library.LastCheckedAt = pgdb.UTC(row.LastCheckedAt)
	return library
}

func domainError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
