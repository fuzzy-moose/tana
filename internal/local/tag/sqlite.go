package tag

import (
	"context"
	"database/sql"

	"github.com/fuzzy-moose/tana/internal/local/tag/dbgen"
)

type SQLiteRepository struct {
	queries *dbgen.Queries
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{queries: dbgen.New(db)}
}

func (r *SQLiteRepository) ListForGallery(ctx context.Context, galleryID int64) ([]Tag, error) {
	rows, err := r.queries.ListGalleryTags(ctx, galleryID)
	if err != nil {
		return nil, err
	}
	tags := make([]Tag, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, Tag{ID: row.ID, Namespace: Namespace{ID: row.NamespaceID, Name: row.NamespaceName}, Value: row.Value})
	}
	return tags, nil
}

// ReplaceForGalleryTx replaces assignments in the caller's transaction. Any
// error requires rollback; shared vocabulary entries retain their identities.
func (r *SQLiteRepository) ReplaceForGalleryTx(ctx context.Context, tx *sql.Tx, galleryID int64, values []Value) error {
	q := r.queries.WithTx(tx)
	if err := q.DeleteGalleryTags(ctx, galleryID); err != nil {
		return err
	}
	for _, value := range values {
		value, err := Normalize(value)
		if err != nil {
			return err
		}
		namespace, err := q.EnsureNamespace(ctx, value.Namespace)
		if err != nil {
			return err
		}
		t, err := q.EnsureTag(ctx, dbgen.EnsureTagParams{NamespaceID: namespace.ID, Value: value.Value})
		if err != nil {
			return err
		}
		if err := q.AssignGalleryTag(ctx, dbgen.AssignGalleryTagParams{GalleryID: galleryID, TagID: t.ID}); err != nil {
			return err
		}
	}
	return nil
}
