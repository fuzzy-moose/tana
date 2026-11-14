package catalog

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

var ErrInvalidVerdict = errors.New("invalid verdict")

// SetVerdict stores the user's verdict on a gallery and records a display of
// it in one statement, so a gallery with a verdict is always displayed. A
// different verdict replaces the stored one and its time; the same verdict
// keeps the row. IDs need no catalog row, as for RecordDisplays (ADR 0019).
func (s *Service) SetVerdict(ctx context.Context, galleryID int64, verdict string) error {
	if galleryID <= 0 {
		return ErrInvalidGalleryReference
	}
	if !collectorapi.ValidVerdict(verdict) {
		return ErrInvalidVerdict
	}
	_, err := s.db.Exec(ctx, `WITH verdict AS (
			INSERT INTO panda_gallery_verdicts (gallery_id, verdict, given_at) VALUES ($1, $2, now())
			ON CONFLICT (gallery_id) DO UPDATE SET verdict = excluded.verdict, given_at = excluded.given_at
			WHERE panda_gallery_verdicts.verdict <> excluded.verdict
		)
		INSERT INTO panda_gallery_displays (gallery_id, displayed_at) VALUES ($1, now())
		ON CONFLICT DO NOTHING`, galleryID, verdict)
	return err
}

// RemoveVerdict deletes the gallery's verdict, if any. Its displays stay.
func (s *Service) RemoveVerdict(ctx context.Context, galleryID int64) error {
	if galleryID <= 0 {
		return ErrInvalidGalleryReference
	}
	_, err := s.db.Exec(ctx, "DELETE FROM panda_gallery_verdicts WHERE gallery_id = $1", galleryID)
	return err
}

// markVerdicts looks up only the page's items, like markDisplayed.
func markVerdicts(ctx context.Context, tx pgx.Tx, items []collectorapi.CatalogItem) error {
	byID := make(map[int64]*collectorapi.CatalogItem, len(items))
	ids := make([]int64, 0, len(items))
	for i := range items {
		byID[items[i].GalleryID] = &items[i]
		ids = append(ids, items[i].GalleryID)
	}
	rows, err := tx.Query(ctx, "SELECT gallery_id, verdict FROM panda_gallery_verdicts WHERE gallery_id = ANY($1::bigint[])", ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var verdict string
		if err := rows.Scan(&id, &verdict); err != nil {
			return err
		}
		byID[id].Verdict = verdict
	}
	return rows.Err()
}

// verdictPageQuery starts from the few galleries with the verdict. The join
// stays materialized, without the limit, so the planner can't turn it into an
// ordered catalog scan that probes the verdict table for every gallery.
func verdictPageQuery(predicate string, args *pgdb.Args, verdict string, before bool, limit int) string {
	order := "DESC"
	if before {
		order = "ASC"
	}
	return `WITH judged AS MATERIALIZED (
		SELECT g.gallery_id, g.ordered_at FROM panda_gallery_verdicts v
		JOIN catalog_galleries g ON g.gallery_id = v.gallery_id
		WHERE v.verdict = ` + args.Add(verdict) + ` AND ` + predicate + `
	) SELECT gallery_id, ordered_at FROM judged ORDER BY ordered_at ` + order + `, gallery_id ` + order + ` LIMIT ` + args.Add(limit)
}
