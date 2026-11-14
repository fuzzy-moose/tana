package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

var ErrInvalidDisplays = errors.New("invalid catalog displays")
var ErrGalleryNotInCatalog = errors.New("gallery not in catalog")

// RecordDisplays stores one display per distinct gallery of a listing page, all
// at the collector's time of the record. A display at the same instant as an
// existing one adds nothing, so it is dropped. IDs need no catalog row: the seen
// state outlives every projection change (ADR 0019). scoreFilter is the filter
// state of the listing: a display in the "hidden only" state is seen but is not
// evidence for the score model.
func (s *Service) RecordDisplays(ctx context.Context, ids []int64, scoreFilter string) error {
	if len(ids) == 0 || len(ids) > collectorapi.MaxCatalogDisplays || !collectorapi.ValidScoreFilter(scoreFilter) {
		return ErrInvalidDisplays
	}
	for _, id := range ids {
		if id <= 0 {
			return ErrInvalidDisplays
		}
	}
	_, err := s.db.Exec(ctx, `INSERT INTO panda_gallery_displays (gallery_id, displayed_at, evidence)
		SELECT DISTINCT id, now(), $2::boolean FROM unnest($1::bigint[]) AS ids(id)
		ON CONFLICT DO NOTHING`, ids, scoreFilter != collectorapi.ScoreFilterHidden)
	return err
}

// CatchUp moves the catch-up position to the gallery's current catalog
// position when that is newer. The conflict update locks the singleton row and
// re-checks its condition on the latest version, so concurrent catch-ups only
// ever advance the position.
func (s *Service) CatchUp(ctx context.Context, galleryID int64) error {
	if galleryID <= 0 {
		return ErrInvalidGalleryReference
	}
	var found bool
	err := s.db.QueryRow(ctx, `WITH g AS (
			SELECT ordered_at, gallery_id FROM catalog_galleries WHERE gallery_id = $1
		), caught_up AS (
			INSERT INTO panda_catalog_catch_up (id, ordered_at, gallery_id, caught_up_at)
			SELECT 1, ordered_at, gallery_id, now() FROM g
			ON CONFLICT (id) DO UPDATE SET ordered_at = excluded.ordered_at, gallery_id = excluded.gallery_id,
				caught_up_at = excluded.caught_up_at
			WHERE (panda_catalog_catch_up.ordered_at, panda_catalog_catch_up.gallery_id) < (excluded.ordered_at, excluded.gallery_id)
		)
		SELECT EXISTS (SELECT 1 FROM g)`, galleryID).Scan(&found)
	if err != nil {
		return err
	}
	if !found {
		return ErrGalleryNotInCatalog
	}
	return nil
}

// catchUpPosition returns the stored catch-up position, if any.
func catchUpPosition(ctx context.Context, tx pgx.Tx) (orderedAt time.Time, galleryID int64, ok bool, err error) {
	err = tx.QueryRow(ctx, "SELECT ordered_at, gallery_id FROM panda_catalog_catch_up WHERE id = 1").Scan(&orderedAt, &galleryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, 0, false, nil
	}
	return orderedAt, galleryID, err == nil, err
}

// markDisplayed looks up only the page's items, never every candidate.
func markDisplayed(ctx context.Context, tx pgx.Tx, items []collectorapi.CatalogItem) error {
	byID := make(map[int64]*collectorapi.CatalogItem, len(items))
	ids := make([]int64, 0, len(items))
	for i := range items {
		byID[items[i].GalleryID] = &items[i]
		ids = append(ids, items[i].GalleryID)
	}
	rows, err := tx.Query(ctx, `SELECT id FROM unnest($1::bigint[]) AS ids(id)
		WHERE EXISTS (SELECT 1 FROM panda_gallery_displays d WHERE d.gallery_id = ids.id)`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		byID[id].Displayed = true
	}
	return rows.Err()
}
