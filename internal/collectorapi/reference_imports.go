package collectorapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const MaxReferenceImportBytes int64 = 100 << 20

type ReferenceImport struct {
	ID             string     `json:"id"`
	Filename       string     `json:"filename"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	SizeBytes      int64      `json:"size_bytes"`
	ProcessedBytes int64      `json:"processed_bytes"`
	References     int64      `json:"references"`
	Duplicates     int64      `json:"duplicates"`
	Invalid        int64      `json:"invalid"`
	Known          int64      `json:"known"`
	Imported       int64      `json:"imported"`
	Failed         int64      `json:"failed"`
	Pending        int64      `json:"pending"`
	Cancelled      int64      `json:"cancelled"`
}

type ReferenceImportList struct {
	Imports []ReferenceImport `json:"imports"`
}

// SubmitReferenceImport streams a complete JSONL file before the collector accepts
// it. Uploads have no total transfer timeout; cancellation follows the caller.
func (c *Client) SubmitReferenceImport(ctx context.Context, filename string, body io.Reader) (ReferenceImport, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/api/reference-imports?filename="+url.QueryEscape(filename), body)
	if err != nil {
		return ReferenceImport{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("Accept", "application/json")
	response, err := c.files.Do(req)
	if err != nil {
		return ReferenceImport{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		return ReferenceImport{}, downloadResponseError(response)
	}
	var result ReferenceImport
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result)
	return result, err
}

func (c *Client) ListReferenceImports(ctx context.Context, limit, offset int64) (ReferenceImportList, error) {
	var result ReferenceImportList
	err := c.downloadRequest(ctx, http.MethodGet, fmt.Sprintf("/api/reference-imports?limit=%d&offset=%d", limit, offset), nil, &result)
	if err == nil && result.Imports == nil {
		err = fmt.Errorf("collector returned an invalid reference import list")
	}
	return result, err
}

func referenceImportPath(id string) string { return "/api/reference-imports/" + url.PathEscape(id) }

func (c *Client) GetReferenceImport(ctx context.Context, id string) (ReferenceImport, error) {
	var result ReferenceImport
	err := c.downloadRequest(ctx, http.MethodGet, referenceImportPath(id), nil, &result)
	return result, err
}

func (c *Client) CancelReferenceImport(ctx context.Context, id string) (ReferenceImport, error) {
	var result ReferenceImport
	err := c.downloadRequest(ctx, http.MethodPost, referenceImportPath(id)+"/cancel", nil, &result)
	return result, err
}

func (c *Client) RetryReferenceImport(ctx context.Context, id string) (ReferenceImport, error) {
	var result ReferenceImport
	err := c.downloadRequest(ctx, http.MethodPost, referenceImportPath(id)+"/retry", nil, &result)
	return result, err
}
