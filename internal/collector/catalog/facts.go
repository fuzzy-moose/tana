package catalog

import (
	"context"
	"errors"
	"strings"

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
	args := make([]any, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrInvalidFactsBatch
		}
		if !seen[id] {
			args = append(args, id)
			seen[id] = true
		}
	}
	rows, err := s.db.QueryContext(ctx, `WITH requested(gallery_id) AS (VALUES (?)`+strings.Repeat(",(?)", len(args)-1)+`)
		SELECT r.gallery_id, COALESCE(g.category, ''),
			(SELECT MAX(f.added_at) FROM favorites f WHERE f.gallery_id = r.gallery_id)
		FROM requested r LEFT JOIN catalog_galleries g ON g.gallery_id = r.gallery_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fact collectorapi.CatalogFact
		if err := rows.Scan(&fact.GalleryID, &fact.Category, &fact.FavoritedAt); err != nil {
			return nil, err
		}
		result = append(result, fact)
	}
	return result, rows.Err()
}
