package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/panda"
)

// Whole favorite categories can exceed the ordinary API request/response limits.
const MaxDownloadBatchBytes = 64 << 20

type FavoriteDownloadCandidate struct {
	Ref   panda.GalleryRef `json:"ref"`
	State string           `json:"state"`
}

type DownloadBatchCounts struct {
	NewDownloads     int64 `json:"new_downloads"`
	ExistingJobs     int64 `json:"existing_jobs"`
	RetainedArchives int64 `json:"retained_archives"`
	Failed           int64 `json:"failed"`
	Cancelled        int64 `json:"cancelled"`
	Deleting         int64 `json:"deleting"`
}

func (c *Client) FavoriteDownloadCandidates(ctx context.Context, category int) ([]FavoriteDownloadCandidate, error) {
	if category < 0 || category > 9 {
		return nil, ErrInvalidCategory
	}
	response, err := c.controlRequest(ctx, http.MethodGet, "/api/favorites/"+strconv.Itoa(category)+"/download-candidates", nil)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, downloadResponseError(response)
	}
	var result struct {
		Favorites []FavoriteDownloadCandidate `json:"favorites"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, MaxDownloadBatchBytes)).Decode(&result); err != nil {
		return nil, err
	}
	if result.Favorites == nil {
		return nil, fmt.Errorf("collector returned an invalid favorite download candidate list")
	}
	return result.Favorites, nil
}

func (c *Client) SubmitDownloads(ctx context.Context, refs []panda.GalleryRef) (DownloadBatchCounts, error) {
	body, err := json.Marshal(struct {
		References []panda.GalleryRef `json:"references"`
	}{refs})
	if err != nil {
		return DownloadBatchCounts{}, err
	}
	var result DownloadBatchCounts
	err = c.downloadRequest(ctx, http.MethodPost, "/api/downloads/batch", bytes.NewReader(body), &result)
	return result, err
}
