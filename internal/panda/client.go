// Package panda provides gallery metadata.
package panda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const MaxBatchSize = 25

type Client struct {
	httpClient *http.Client
	apiURL     string
}

// NewClient requires an absolute HTTP(S) API URL and uses http.DefaultClient when
// httpClient is nil. Callers control timeouts, pacing, and retries.
func NewClient(apiURL string, httpClient *http.Client) (*Client, error) {
	if err := validateAPIURL(apiURL); err != nil {
		return nil, fmt.Errorf("panda: API URL: %w", err)
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{httpClient: httpClient, apiURL: apiURL}, nil
}

// GalleryRef identifies an upstream gallery independently of Tana's catalog.
type GalleryRef struct {
	ID    int64  `json:"gid"`
	Token string `json:"token"`
}

// HTTPError exposes the upstream status and raw Retry-After header to callers.
type HTTPError struct {
	StatusCode int
	RetryAfter string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("panda: HTTP %d", e.StatusCode)
}

// GetMetadata fetches one batch of 1–25 galleries with namespaced tags.
// Results retain upstream order and individual lookup errors in Metadata.Error;
// the returned error describes a request or response failure affecting the batch.
func (c *Client) GetMetadata(ctx context.Context, galleries []GalleryRef) ([]Metadata, error) {
	if len(galleries) == 0 || len(galleries) > MaxBatchSize {
		return nil, fmt.Errorf("panda: batch must contain 1–%d galleries", MaxBatchSize)
	}
	request := struct {
		Method    string   `json:"method"`
		GIDList   [][2]any `json:"gidlist"`
		Namespace int      `json:"namespace"`
	}{Method: "gdata", Namespace: 1, GIDList: make([][2]any, len(galleries))}
	for i, gallery := range galleries {
		if gallery.ID <= 0 || gallery.Token == "" {
			return nil, fmt.Errorf("panda: gallery at index %d requires a positive ID and nonempty token", i)
		}
		request.GIDList[i] = [2]any{gallery.ID, gallery.Token}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("panda: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("panda: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Tana")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("panda: request metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, RetryAfter: resp.Header.Get("Retry-After")}
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("panda: read response: %w", err)
	}
	var response struct {
		Metadata []Metadata `json:"gmetadata"`
		Error    string     `json:"error"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("panda: decode response: %w", err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("panda: API error: %s", response.Error)
	}
	if len(response.Metadata) != len(galleries) {
		return nil, fmt.Errorf("panda: expected %d metadata entries, got %d", len(galleries), len(response.Metadata))
	}
	return response.Metadata, nil
}
