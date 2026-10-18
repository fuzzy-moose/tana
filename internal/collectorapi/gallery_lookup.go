package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type GalleryLookupResult struct {
	GalleryID   int64             `json:"gid"`
	Token       string            `json:"token"`
	URL         string            `json:"url"`
	Metadata    *panda.Metadata   `json:"metadata,omitempty"`
	RefreshedAt *time.Time        `json:"refreshed_at,omitempty"`
	Unverified  bool              `json:"unverified,omitempty"`
	FetchJob    *MetadataFetchJob `json:"fetch_job,omitempty"`
}

func (c *Client) LookupGallery(ctx context.Context, ref panda.GalleryRef) (GalleryLookupResult, error) {
	body, err := json.Marshal(ref)
	if err != nil {
		return GalleryLookupResult{}, err
	}
	response, err := c.controlRequest(ctx, http.MethodPost, "/api/catalog/lookup", bytes.NewReader(body))
	if err != nil {
		return GalleryLookupResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return GalleryLookupResult{}, downloadResponseError(response)
	}
	var result GalleryLookupResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&result); err != nil {
		return GalleryLookupResult{}, fmt.Errorf("decode collector gallery lookup: %w", err)
	}
	if result.GalleryID != ref.ID || result.Token == "" || result.URL == "" ||
		(result.Metadata != nil && (result.Metadata.ID != ref.ID || result.Metadata.Error != "" || result.RefreshedAt == nil)) ||
		(result.Unverified && (result.FetchJob == nil || result.Token != ref.Token)) ||
		(result.FetchJob != nil && (len(result.FetchJob.Entries) != 1 || result.FetchJob.Entries[0].GalleryID != ref.ID || !validMetadataFetchJob(*result.FetchJob))) {
		return GalleryLookupResult{}, fmt.Errorf("collector returned an invalid gallery lookup")
	}
	return result, nil
}

func (c *Client) GetMetadataFetch(ctx context.Context, id string) (MetadataFetchJob, error) {
	var job MetadataFetchJob
	err := c.catalogRequest(ctx, "/api/metadata/fetches/"+url.PathEscape(id), &job)
	if err == nil && (job.ID != id || !validMetadataFetchJob(job)) {
		err = fmt.Errorf("collector returned an invalid metadata fetch job")
	}
	return job, err
}

func validMetadataFetchJob(job MetadataFetchJob) bool {
	if job.ID == "" || job.CreatedAt.IsZero() || len(job.Entries) == 0 ||
		(job.Status != "pending" && job.Status != "completed") {
		return false
	}
	for _, entry := range job.Entries {
		if entry.GalleryID <= 0 || (entry.Status != "pending" && entry.Status != "successful" && entry.Status != "failed") {
			return false
		}
	}
	return true
}
