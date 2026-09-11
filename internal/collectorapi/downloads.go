package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type DownloadJob struct {
	GalleryID int64      `json:"gallery_id"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	RetryAt   *time.Time `json:"retry_at,omitempty"`
	Failures  int64      `json:"failures"`
	SizeBytes int64      `json:"size_bytes"`
	Error     string     `json:"error,omitempty"`
}

type DownloadList struct {
	Jobs []DownloadJob `json:"jobs"`
}

func (c *Client) ListDownloads(ctx context.Context, limit, offset int64) (DownloadList, error) {
	var result DownloadList
	err := c.downloadRequest(ctx, http.MethodGet, fmt.Sprintf("/api/downloads?limit=%d&offset=%d", limit, offset), nil, &result)
	if err == nil && result.Jobs == nil {
		err = fmt.Errorf("collector returned an invalid download list")
	}
	return result, err
}

func (c *Client) SubmitDownload(ctx context.Context, ref panda.GalleryRef) (DownloadJob, error) {
	body, err := json.Marshal(ref)
	if err != nil {
		return DownloadJob{}, err
	}
	var job DownloadJob
	err = c.downloadRequest(ctx, http.MethodPost, "/api/downloads", bytes.NewReader(body), &job)
	return job, err
}

func downloadPath(id int64) string { return "/api/downloads/" + strconv.FormatInt(id, 10) }

func (c *Client) GetDownload(ctx context.Context, id int64) (DownloadJob, error) {
	var job DownloadJob
	err := c.downloadRequest(ctx, http.MethodGet, downloadPath(id), nil, &job)
	return job, err
}

func (c *Client) RetryDownload(ctx context.Context, id int64) (DownloadJob, error) {
	var job DownloadJob
	err := c.downloadRequest(ctx, http.MethodPost, downloadPath(id)+"/retry", nil, &job)
	return job, err
}

func (c *Client) CancelDownload(ctx context.Context, id int64) (DownloadJob, error) {
	var job DownloadJob
	err := c.downloadRequest(ctx, http.MethodPost, downloadPath(id)+"/cancel", nil, &job)
	return job, err
}

func (c *Client) DeleteDownload(ctx context.Context, id int64) error {
	return c.downloadRequest(ctx, http.MethodDelete, downloadPath(id), nil, nil)
}

func (c *Client) downloadRequest(ctx context.Context, method, path string, body io.Reader, result any) error {
	response, err := c.controlRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted && response.StatusCode != http.StatusNoContent {
		return downloadResponseError(response)
	}
	if result == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(result)
}

func downloadResponseError(response *http.Response) error {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&body)
	return &HTTPError{StatusCode: response.StatusCode, Code: body.Error}
}

// OpenDownload streams with a bounded header wait but no total transfer timeout.
// Only range headers are forwarded; browser credentials never reach the collector.
// The caller owns the returned response body.
func (c *Client) OpenDownload(ctx context.Context, id int64, method string, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+downloadPath(id)+"/file", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	for _, name := range []string{"Range", "If-Range"} {
		if value := headers.Get(name); value != "" {
			req.Header.Set(name, value)
		}
	}
	response, err := c.files.Do(req)
	if err != nil {
		return nil, err
	}
	switch response.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusRequestedRangeNotSatisfiable:
		return response, nil
	default:
		defer response.Body.Close()
		return nil, downloadResponseError(response)
	}
}
