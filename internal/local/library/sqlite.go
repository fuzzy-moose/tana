package library

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/library/dbgen"
)

// SQLiteRepository persists libraries using an application-owned database.
type SQLiteRepository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db, queries: dbgen.New(db)}
}

func (r *SQLiteRepository) Create(ctx context.Context, name, root string) (Library, error) {
	// An immediate SQLite transaction serializes validation and insertion,
	// including registrations through another connection to the same database.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Library{}, err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx)
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
		LastCheckedAt: sql.NullInt64{Int64: time.Now().UnixMilli(), Valid: true},
	})
	if err != nil {
		return Library{}, err
	}
	if err := tx.Commit(); err != nil {
		return Library{}, err
	}
	return fromRow(row), nil
}

func (r *SQLiteRepository) List(ctx context.Context) ([]Library, error) {
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

func (r *SQLiteRepository) Get(ctx context.Context, id int64) (Library, error) {
	row, err := r.queries.GetLibrary(ctx, id)
	return fromRow(row), domainError(err)
}

func (r *SQLiteRepository) Rename(ctx context.Context, id int64, name string) (Library, error) {
	row, err := r.queries.RenameLibrary(ctx, dbgen.RenameLibraryParams{ID: id, Name: name})
	return fromRow(row), domainError(err)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id int64) error {
	// The database owns library records; this operation never visits the root.
	return r.queries.DeleteLibrary(ctx, id)
}

func (r *SQLiteRepository) ResetAvailability(ctx context.Context) error {
	return r.queries.ResetAvailability(ctx)
}

func (r *SQLiteRepository) UpdateAvailability(ctx context.Context, id int64, availability string, checkedAt time.Time) error {
	return r.queries.UpdateAvailability(ctx, dbgen.UpdateAvailabilityParams{
		ID: id, Availability: availability,
		LastCheckedAt: sql.NullInt64{Int64: checkedAt.UnixMilli(), Valid: true},
	})
}

func fromRow(row dbgen.Library) Library {
	library := Library{ID: row.ID, Name: row.Name, Path: row.Path, Availability: row.Availability}
	if row.LastCheckedAt.Valid {
		t := time.UnixMilli(row.LastCheckedAt.Int64).UTC()
		library.LastCheckedAt = &t
	}
	return library
}

func domainError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
