package collectorapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Status struct {
	Available bool `json:"available"`
}

type MetadataStatus struct {
	MetadataErrors        []MetadataError `json:"metadata_errors"`
	MetadataLastError     string          `json:"metadata_last_error,omitempty"`
	MetadataRetryAt       *time.Time      `json:"metadata_retry_at,omitempty"`
	UpstreamCooldownUntil *time.Time      `json:"upstream_cooldown_until,omitempty"`
}

func (c *Client) FavoritesStatus(ctx context.Context) (FavoritesStatus, error) {
	var result FavoritesStatus
	err := c.readStatus(ctx, "/api/favorites/status", &result)
	if err == nil {
		if len(result.Categories) != 10 {
			return result, fmt.Errorf("collector returned an invalid favorites status")
		}
		for i, category := range result.Categories {
			if category.Category != i {
				return result, fmt.Errorf("collector returned an invalid favorites status")
			}
		}
	}
	return result, err
}

func (c *Client) InventoryStatus(ctx context.Context) (InventoryStatus, error) {
	var result InventoryStatus
	err := c.readStatus(ctx, "/api/inventory/status", &result)
	return result, err
}

func (c *Client) MetadataStatus(ctx context.Context) (MetadataStatus, error) {
	var result MetadataStatus
	err := c.readStatus(ctx, "/api/metadata/status", &result)
	if err == nil && result.MetadataErrors == nil {
		err = fmt.Errorf("collector returned an invalid metadata status")
	}
	return result, err
}

func (c *Client) readStatus(ctx context.Context, path string, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	response, err := c.controlRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
}

type FavoritesStatus struct {
	Host       string                  `json:"host"`
	AccountKey string                  `json:"account_key"`
	Categories []FavoriteCategory      `json:"categories"`
	Downloads  FavoriteDownloadsStatus `json:"downloads"`
}

type FavoriteDownloadsStatus struct {
	FavoriteDownloadSettings
	BaselineState      string `json:"baseline_state"`
	BaselineCategories int    `json:"baseline_categories"`
}

type FavoriteCategory struct {
	Category     int        `json:"category"`
	Name         string     `json:"name"`
	Favorites    int64      `json:"favorites"`
	EntriesSaved int64      `json:"entries_saved"`
	PagesSaved   int64      `json:"pages_saved"`
	LastSavedAt  *time.Time `json:"last_saved_at,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	State        string     `json:"state"`
	Full         bool       `json:"full"`
	Queued       bool       `json:"queued"`
	QueuedFull   bool       `json:"queued_full"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	LastOutcome  string     `json:"last_outcome,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	LastErrorAt  *time.Time `json:"last_error_at,omitempty"`
	RetryAt      *time.Time `json:"retry_at,omitempty"`
}

// InventoryStatus Inventory metadata counts partition confirmed gallery references.
// Explicit fetch requests can also contain unconfirmed references or refresh retained data.
type InventoryStatus struct {
	GalleryReferences int64 `json:"gallery_references"`
	MetadataAvailable int64 `json:"metadata_available"`
	MetadataPending   int64 `json:"metadata_pending"`
	MetadataFailed    int64 `json:"metadata_failed"`
	FetchesPending    int64 `json:"fetches_pending"`
	FetchesFailed     int64 `json:"fetches_failed"`
}

type MetadataError struct {
	GalleryID int64      `json:"gallery_id"`
	Error     string     `json:"error"`
	At        *time.Time `json:"at,omitempty"`
}
