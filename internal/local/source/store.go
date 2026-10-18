package source

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/local/source/dbgen"
)

type Store struct {
	db      *pgxpool.Pool
	queries *dbgen.Queries
}

var _ Repository = (*Store)(nil)

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, queries: dbgen.New(db)}
}

func (r *Store) Create(ctx context.Context, libraryID int64, path string, kind Kind, files []string) (Source, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback(ctx)
	s, err := r.CreateTx(ctx, tx, libraryID, path, kind, files)
	if err != nil {
		return Source{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Source{}, err
	}
	return s, nil
}

// CreateTx registers an inventory in the caller's transaction. The caller must
// roll back the transaction on error.
func (r *Store) CreateTx(ctx context.Context, tx pgx.Tx, libraryID int64, path string, kind Kind, files []string) (Source, error) {
	if err := validate(path, kind, files); err != nil {
		return Source{}, err
	}
	q := r.queries.WithTx(tx)
	row, err := q.CreateSource(ctx, dbgen.CreateSourceParams{LibraryID: libraryID, Path: path, Kind: string(kind)})
	if err != nil {
		return Source{}, err
	}
	for _, name := range files {
		if err := q.CreateSourceFile(ctx, dbgen.CreateSourceFileParams{SourceID: row.ID, Path: name}); err != nil {
			return Source{}, err
		}
	}
	return fromRow(row), nil
}

func (r *Store) Get(ctx context.Context, id int64) (Source, error) {
	row, err := r.queries.GetSource(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return fromRow(row), err
}

func (r *Store) List(ctx context.Context, libraryID int64) ([]Source, error) {
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

func (r *Store) Files(ctx context.Context, id int64) ([]File, error) {
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

func (r *Store) Delete(ctx context.Context, id int64) error {
	return r.queries.DeleteSource(ctx, id)
}

func fromRow(row dbgen.Source) Source {
	return Source{ID: row.ID, LibraryID: row.LibraryID, Path: row.Path, Kind: Kind(row.Kind)}
}
