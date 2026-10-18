package collectorapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxDownloadSnapshotBytes = 64 << 20

func (c *Client) CompletedDownloadIDs(ctx context.Context) ([]int64, error) {
	response, err := c.controlRequest(ctx, http.MethodGet, "/api/downloads/completed", nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, downloadResponseError(response)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDownloadSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxDownloadSnapshotBytes {
		return nil, fmt.Errorf("collector completed download snapshot is too large")
	}
	var result struct {
		GalleryIDs []int64 `json:"gallery_ids"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode collector completed download snapshot: %w", err)
	}
	if result.GalleryIDs == nil {
		return nil, fmt.Errorf("collector returned an invalid completed download snapshot")
	}
	seen := make(map[int64]struct{}, len(result.GalleryIDs))
	for _, id := range result.GalleryIDs {
		if id <= 0 {
			return nil, fmt.Errorf("collector completed download snapshot returned invalid ID %d", id)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("collector completed download snapshot returned duplicate ID %d", id)
		}
		seen[id] = struct{}{}
	}
	return result.GalleryIDs, nil
}
