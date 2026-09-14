package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const MaxCatalogFactsSize = 1000

type CatalogFact struct {
	GalleryID   int64  `json:"gallery_id"`
	Category    string `json:"category"`
	FavoritedAt *int64 `json:"favorited_at"`
}

// CatalogFacts returns each distinct requested gallery, including IDs without
// collected data. Larger requests are split into bounded collector batches.
func (c *Client) CatalogFacts(ctx context.Context, ids []int64) ([]CatalogFact, error) {
	unique := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, fmt.Errorf("invalid gallery ID %d", id)
		}
		if !seen[id] {
			unique = append(unique, id)
			seen[id] = true
		}
	}
	result := make([]CatalogFact, 0, len(unique))
	for start := 0; start < len(unique); start += MaxCatalogFactsSize {
		batch, err := c.catalogFactsBatch(ctx, unique[start:min(start+MaxCatalogFactsSize, len(unique))])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (c *Client) catalogFactsBatch(ctx context.Context, ids []int64) ([]CatalogFact, error) {
	body, err := json.Marshal(struct {
		GalleryIDs []int64 `json:"gallery_ids"`
	}{ids})
	if err != nil {
		return nil, err
	}
	response, err := c.controlRequest(ctx, http.MethodPost, "/api/catalog/facts", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, downloadResponseError(response)
	}
	var result []CatalogFact
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return nil, err
	}
	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	for _, fact := range result {
		if !wanted[fact.GalleryID] {
			return nil, fmt.Errorf("collector catalog facts returned unexpected or duplicate ID %d", fact.GalleryID)
		}
		delete(wanted, fact.GalleryID)
	}
	if len(wanted) != 0 {
		return nil, fmt.Errorf("collector catalog facts omitted %d IDs", len(wanted))
	}
	return result, nil
}
