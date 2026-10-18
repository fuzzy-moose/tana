package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

var ErrInvalidFactsBatch = errors.New("invalid catalog facts batch")

// Facts joins categories and current favorite membership independently: a
// favorite timestamp does not require collected metadata.
func (s *Service) Facts(ctx context.Context, ids []int64) ([]collectorapi.CatalogFact, error) {
	if len(ids) > collectorapi.MaxCatalogFactsSize {
		return nil, ErrInvalidFactsBatch
	}
	result := make([]collectorapi.CatalogFact, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	requested := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidFactsBatch
		}
		if !seen[id] {
			requested = append(requested, id)
			seen[id] = true
		}
	}
	rows, err := s.db.Query(ctx, `SELECT r.gallery_id, COALESCE(g.category, ''),
			(SELECT MAX(f.added_at) FROM favorites f WHERE f.gallery_id = r.gallery_id)
		FROM unnest($1::bigint[]) WITH ORDINALITY AS r(gallery_id, position)
		LEFT JOIN catalog_galleries g ON g.gallery_id = r.gallery_id
		ORDER BY r.position`, requested)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fact collectorapi.CatalogFact
		var favoritedAt *time.Time
		if err := rows.Scan(&fact.GalleryID, &fact.Category, &favoritedAt); err != nil {
			return nil, err
		}
		if favoritedAt != nil {
			// The API keeps Unix seconds, the favorites' native precision.
			seconds := favoritedAt.Unix()
			fact.FavoritedAt = &seconds
		}
		result = append(result, fact)
	}
	return result, rows.Err()
}
