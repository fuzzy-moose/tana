package favorites

import (
	"context"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func (s *Service) DownloadCandidates(ctx context.Context, category int) ([]collectorapi.FavoriteDownloadCandidate, error) {
	if category < 0 || category > 9 {
		return nil, ErrInvalidCategory
	}
	rows, err := s.store.q.DownloadCandidates(ctx, dbgen.DownloadCandidatesParams{
		Host: s.store.host, AccountKey: s.store.accountKey, Category: int64(category),
	})
	if err != nil {
		return nil, err
	}
	result := make([]collectorapi.FavoriteDownloadCandidate, 0, len(rows))
	for _, row := range rows {
		result = append(result, collectorapi.FavoriteDownloadCandidate{
			Ref: panda.GalleryRef{ID: row.GalleryID, Token: row.Token}, State: row.State,
		})
	}
	return result, nil
}
