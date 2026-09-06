package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

var ErrInvalidCategory = errors.New("invalid_category")

type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("collector returned HTTP %d", e.StatusCode) }

type ConnectionStatus struct {
	Configured    bool      `json:"configured"`
	Reachable     bool      `json:"reachable"`
	Authenticated *bool     `json:"authenticated"`
	CheckedAt     time.Time `json:"checked_at"`
	Error         string    `json:"error,omitempty"`
	Status        *Status   `json:"status,omitempty"`
}

// Status checks the authenticated API; a health response alone cannot prove
// access to statistics. Connection failures are data for the local status page.
func (c *Client) Status(ctx context.Context) ConnectionStatus {
	result := ConnectionStatus{Configured: true}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	response, err := c.controlRequest(ctx, http.MethodGet, "/api/status", nil)
	result.CheckedAt = time.Now().UTC()
	if err != nil {
		result.Error = "collector_unreachable"
		return result
	}
	defer response.Body.Close()
	result.Reachable = true
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		authenticated := false
		result.Authenticated = &authenticated
		result.Error = "collector_unauthorized"
		return result
	}
	if response.StatusCode != http.StatusOK {
		result.Error = "collector_status_unavailable"
		return result
	}
	var status Status
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status); err != nil || len(status.Favorites.Categories) != 10 {
		result.Error = "collector_invalid_response"
		return result
	}
	for i, category := range status.Favorites.Categories {
		if category.Category != i {
			result.Error = "collector_invalid_response"
			return result
		}
	}
	authenticated := true
	result.Authenticated = &authenticated
	result.Status = &status
	return result
}

func (c *Client) SyncFavorites(ctx context.Context, category string, full bool) error {
	if category != "all" {
		id, err := strconv.Atoi(category)
		if err != nil || id < 0 || id > 9 {
			return ErrInvalidCategory
		}
		category = strconv.Itoa(id)
	}
	body, err := json.Marshal(struct {
		Full bool `json:"full"`
	}{full})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	response, err := c.controlRequest(ctx, http.MethodPost, "/api/favorites/"+category+"/sync", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return nil
}

func (c *Client) controlRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}
