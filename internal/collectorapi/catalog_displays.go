package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// MaxCatalogDisplays bounds one record of displayed galleries: one listing page.
const MaxCatalogDisplays = 100

// RecordCatalogDisplays records the galleries a Panda listing page displayed in
// the score filter state scoreFilter.
func (c *Client) RecordCatalogDisplays(ctx context.Context, ids []int64, scoreFilter string) error {
	return c.postCatalogSeen(ctx, "/api/catalog/displays", struct {
		GalleryIDs  []int64 `json:"gallery_ids"`
		ScoreFilter string  `json:"score_filter,omitempty"`
	}{ids, scoreFilter})
}

// CatalogCatchUp marks the gallery's catalog position and everything older as
// seen.
func (c *Client) CatalogCatchUp(ctx context.Context, galleryID int64) error {
	return c.postCatalogSeen(ctx, "/api/catalog/catch-up", struct {
		GalleryID int64 `json:"gallery_id"`
	}{galleryID})
}

func (c *Client) postCatalogSeen(ctx context.Context, path string, input any) error {
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	response, err := c.controlRequest(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return downloadResponseError(response)
	}
	return nil
}
