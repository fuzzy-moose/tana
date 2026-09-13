package downloads

import (
	"context"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// SubmitBatch commits every request together. Existing jobs retain their state;
// one invalid reference or conflicting token rejects the entire batch.
func (s *Service) SubmitBatch(ctx context.Context, refs []panda.GalleryRef) (collectorapi.DownloadBatchCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result collectorapi.DownloadBatchCounts
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := s.q.WithTx(tx)
	seen := make(map[int64]string, len(refs))
	for _, ref := range refs {
		if token, ok := seen[ref.ID]; ok {
			if token != ref.Token {
				return collectorapi.DownloadBatchCounts{}, ErrTokenConflict
			}
			continue
		}
		seen[ref.ID] = ref.Token
		inserted, err := enqueue(ctx, q, ref)
		if err != nil {
			return collectorapi.DownloadBatchCounts{}, err
		}
		row, err := q.GetDownload(ctx, ref.ID)
		if err != nil {
			return collectorapi.DownloadBatchCounts{}, err
		}
		if row.Token != ref.Token {
			return collectorapi.DownloadBatchCounts{}, ErrTokenConflict
		}
		if inserted != 0 {
			result.NewDownloads++
			continue
		}
		switch row.State {
		case "queued", "running":
			result.ExistingJobs++
		case "completed":
			result.RetainedArchives++
		case "failed":
			result.Failed++
		case "cancelled":
			result.Cancelled++
		case "deleting":
			result.Deleting++
		}
	}
	if err := tx.Commit(); err != nil {
		return collectorapi.DownloadBatchCounts{}, err
	}
	s.signal()
	return result, nil
}
