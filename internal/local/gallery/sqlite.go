package gallery

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/gallery/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/tag"
)

type SQLiteRepository struct {
	db      *sql.DB
	queries *dbgen.Queries
}

var _ Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db, queries: dbgen.New(db)}
}

func (r *SQLiteRepository) Create(ctx context.Context, title string, fileIDs []string) (Gallery, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Gallery{}, ErrInvalidTitle
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Gallery{}, err
	}
	defer tx.Rollback()
	g, err := create(ctx, r.queries.WithTx(tx), title, fileIDs, "")
	if err != nil {
		return Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return Gallery{}, err
	}
	return g, nil
}

func create(ctx context.Context, q *dbgen.Queries, title string, fileIDs []string, sourceID string) (Gallery, error) {
	row, err := q.CreateGallery(ctx, dbgen.CreateGalleryParams{ID: rand.Text(), Title: title, SourceID: sql.NullString{String: sourceID, Valid: sourceID != ""}})
	if err != nil {
		return Gallery{}, err
	}
	if err := insertPages(ctx, q, row.ID, fileIDs); err != nil {
		return Gallery{}, err
	}
	return fromRow(row), nil
}

func (r *SQLiteRepository) CreateFromSource(ctx context.Context, sourceID string) (Gallery, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Gallery{}, err
	}
	defer tx.Rollback()
	g, err := r.CreateFromSourceTx(ctx, tx, sourceID, metadata.Values{})
	if err != nil {
		return Gallery{}, err
	}
	if err := tx.Commit(); err != nil {
		return Gallery{}, err
	}
	return g, nil
}

// CreateFromSourceTx creates a gallery in the caller's transaction. ErrNoImages
// makes no changes; other errors require the caller to roll back.
func (r *SQLiteRepository) CreateFromSourceTx(ctx context.Context, tx *sql.Tx, sourceID string, values metadata.Values) (Gallery, error) {
	q := r.queries.WithTx(tx)
	s, err := q.GetSourceTitle(ctx, sourceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Gallery{}, source.ErrNotFound
	}
	if err != nil {
		return Gallery{}, err
	}
	files, err := q.ListSourceFilesForGallery(ctx, sourceID)
	if err != nil {
		return Gallery{}, err
	}
	slices.SortFunc(files, func(a, b dbgen.ListSourceFilesForGalleryRow) int {
		if naturalLess(a.Path, b.Path) {
			return -1
		}
		if naturalLess(b.Path, a.Path) {
			return 1
		}
		return 0
	})
	fileIDs := make([]string, 0, len(files))
	for _, file := range files {
		if SupportsImage(file.Path) {
			fileIDs = append(fileIDs, file.ID)
		}
	}
	if len(fileIDs) == 0 {
		return Gallery{}, ErrNoImages
	}
	title := path.Base(s.Path)
	if s.Path == "." {
		title = filepath.Base(s.LibraryPath)
	}
	if source.Kind(s.Kind) == source.Archive {
		title = strings.TrimSuffix(title, path.Ext(title))
	}
	if strings.TrimSpace(values.Title) != "" {
		title = values.Title
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return Gallery{}, ErrInvalidTitle
	}
	g, err := create(ctx, q, title, fileIDs, sourceID)
	if err != nil {
		return Gallery{}, err
	}
	if err := tag.NewSQLiteRepository(r.db).ReplaceForGalleryTx(ctx, tx, g.ID, values.Tags); err != nil {
		return Gallery{}, err
	}
	return g, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id string) (Gallery, error) {
	row, err := r.queries.GetGallery(ctx, id)
	return fromRow(row), domainError(err)
}

func (r *SQLiteRepository) List(ctx context.Context) ([]Gallery, error) {
	rows, err := r.queries.ListGalleries(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Gallery, 0, len(rows))
	for _, row := range rows {
		result = append(result, fromRow(row))
	}
	return result, nil
}

func (r *SQLiteRepository) Pages(ctx context.Context, id string) ([]Page, error) {
	rows, err := r.queries.ListGalleryPages(ctx, id)
	if err != nil {
		return nil, err
	}
	result := make([]Page, 0, len(rows))
	for _, row := range rows {
		result = append(result, Page{GalleryID: row.GalleryID, Number: row.PageNumber, SourceFileID: row.SourceFileID})
	}
	return result, nil
}

func (r *SQLiteRepository) Rename(ctx context.Context, id, title string) (Gallery, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Gallery{}, ErrInvalidTitle
	}
	row, err := r.queries.RenameGallery(ctx, dbgen.RenameGalleryParams{ID: id, Title: title})
	return fromRow(row), domainError(err)
}

func (r *SQLiteRepository) ReplacePages(ctx context.Context, id string, fileIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx)
	g, err := q.GetGallery(ctx, id)
	if err != nil {
		return domainError(err)
	}
	if g.SourceID.Valid {
		pages, err := q.ListGalleryPages(ctx, id)
		if err != nil {
			return err
		}
		if len(pages) != len(fileIDs) {
			return ErrLinkedPages
		}
		remaining := make(map[string]int, len(pages))
		for _, page := range pages {
			remaining[page.SourceFileID]++
		}
		for _, fileID := range fileIDs {
			if remaining[fileID] == 0 {
				return ErrLinkedPages
			}
			remaining[fileID]--
		}
	}
	if err := q.DeleteGalleryPages(ctx, id); err != nil {
		return err
	}
	if err := insertPages(ctx, q, id, fileIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func insertPages(ctx context.Context, q *dbgen.Queries, id string, fileIDs []string) error {
	for i, fileID := range fileIDs {
		name, err := q.GetSourceFilePath(ctx, fileID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", ErrInvalidPage, fileID)
		}
		if err != nil {
			return err
		}
		if !SupportsImage(name) {
			return fmt.Errorf("%w: %s", ErrInvalidPage, fileID)
		}
		if err := q.CreateGalleryPage(ctx, dbgen.CreateGalleryPageParams{GalleryID: id, Position: int64(i + 1), SourceFileID: fileID}); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	return r.queries.DeleteGallery(ctx, id)
}

func fromRow(row dbgen.Gallery) Gallery {
	return Gallery{ID: row.ID, Title: row.Title, SourceID: row.SourceID.String}
}

func domainError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
