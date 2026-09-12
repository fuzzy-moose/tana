package collectorapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

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
