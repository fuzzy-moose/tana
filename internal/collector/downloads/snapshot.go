package downloads

import "context"

// CompletedDownloadIDs captures the entire backlog in one database snapshot.
// Paging the download list could miss archives when completed jobs are deleted.
func (s *Service) CompletedDownloadIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT gallery_id FROM panda_downloads
		WHERE state = 'completed' ORDER BY created_at DESC, gallery_id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
