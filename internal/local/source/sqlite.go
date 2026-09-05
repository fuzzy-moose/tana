package source

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"

	"github.com/fuzzy-moose/tana/internal/local/source/dbgen"
)

type SQLiteRepository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

var _ Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db, queries: dbgen.New(db)}
}

func (r *SQLiteRepository) Create(ctx context.Context, libraryID, path string, kind Kind, files []string) (Source, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	s, err := r.CreateTx(ctx, tx, libraryID, path, kind, files)
	if err != nil {
		return Source{}, err
	}
	if err := tx.Commit(); err != nil {
		return Source{}, err
	}
	return s, nil
}

// CreateTx registers an inventory in the caller's transaction. The caller must
// roll back the transaction on error.
func (r *SQLiteRepository) CreateTx(ctx context.Context, tx *sql.Tx, libraryID, path string, kind Kind, files []string) (Source, error) {
	if err := validate(path, kind, files); err != nil {
		return Source{}, err
	}
	q := r.queries.WithTx(tx)
	row, err := q.CreateSource(ctx, dbgen.CreateSourceParams{ID: rand.Text(), LibraryID: libraryID, Path: path, Kind: string(kind)})
	if err != nil {
		return Source{}, err
	}
	for _, name := range files {
		if err := q.CreateSourceFile(ctx, dbgen.CreateSourceFileParams{ID: rand.Text(), SourceID: row.ID, Path: name}); err != nil {
			return Source{}, err
		}
	}
	return fromRow(row), nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (Source, error) {
	row, err := r.queries.GetSource(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return fromRow(row), err
}

func (r *SQLiteRepository) List(ctx context.Context, libraryID string) ([]Source, error) {
	rows, err := r.queries.ListSources(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	result := make([]Source, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRow(row))
	}
	return result, nil
}

func (r *SQLiteRepository) Files(ctx context.Context, id string) ([]File, error) {
	rows, err := r.queries.ListSourceFiles(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]File, 0, len(rows))
	for _, row := range rows {
		result = append(result, File{ID: row.ID, SourceID: row.SourceID, Path: row.Path})
	}
	return result, nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	return r.queries.DeleteSource(ctx, id)
}

func fromRow(row dbgen.Source) Source {
	return Source{ID: row.ID, LibraryID: row.LibraryID, Path: row.Path, Kind: Kind(row.Kind)}
}
