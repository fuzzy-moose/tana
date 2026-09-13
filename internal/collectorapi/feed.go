package collectorapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FeedCapture struct {
	ID         int64     `json:"id"`
	CapturedAt time.Time `json:"captured_at"`
	SizeBytes  int64     `json:"size_bytes"`
	State      string    `json:"state"`
	Error      string    `json:"error,omitempty"`
}

type FeedCaptureList struct {
	Captures []FeedCapture `json:"captures"`
	HasMore  bool          `json:"has_more"`
}

func (c *Client) ListFeedCaptures(ctx context.Context, failedOnly bool, limit, offset int64) (FeedCaptureList, error) {
	var result FeedCaptureList
	path := fmt.Sprintf("/api/feed/captures?failed_only=%t&limit=%d&offset=%d", failedOnly, limit, offset)
	response, err := c.controlRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, downloadResponseError(response)
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	if err == nil && result.Captures == nil {
		err = fmt.Errorf("collector returned an invalid feed capture list")
	}
	return result, err
}

// OpenFeedCapture returns the original bytes without parsing. The caller owns the response body.
func (c *Client) OpenFeedCapture(ctx context.Context, id int64, method string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+"/api/feed/captures/"+strconv.FormatInt(id, 10)+"/file", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.files.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		defer response.Body.Close()
		return nil, downloadResponseError(response)
	}
	return response, nil
}

type FeedStatus struct {
	CaptureActive      bool       `json:"capture_active"`
	LastCapturedAt     *time.Time `json:"last_captured_at,omitempty"`
	LastCaptureError   string     `json:"last_capture_error,omitempty"`
	LastCaptureErrorAt *time.Time `json:"last_capture_error_at,omitempty"`
	ProcessingPending  int64      `json:"processing_pending"`
	ProcessingError    string     `json:"processing_error,omitempty"`
	Continuity         string     `json:"continuity"`
	PossibleGaps       int64      `json:"possible_gaps"`
}

func (c *Client) FeedStatus(ctx context.Context) (FeedStatus, error) {
	return c.feedRequest(ctx, http.MethodGet, "status")
}

func (c *Client) RefreshFeed(ctx context.Context) (FeedStatus, error) {
	return c.feedRequest(ctx, http.MethodPost, "refresh")
}

func (c *Client) feedRequest(ctx context.Context, method, action string) (FeedStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var result FeedStatus
	var body io.Reader
	if method == http.MethodPost {
		body = strings.NewReader(`{}`)
	}
	response, err := c.controlRequest(ctx, method, "/api/feed/"+action, body)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return result, &HTTPError{StatusCode: response.StatusCode}
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	return result, err
}
