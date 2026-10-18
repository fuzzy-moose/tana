package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type library struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type scanStatus struct {
	Phase            string `json:"phase"`
	Discovered       int    `json:"discovered"`
	Imported         int    `json:"imported"`
	FailedSources    int    `json:"failed_sources"`
	GalleriesCreated int    `json:"galleries_created"`
}

type client struct {
	base string
	http *http.Client
}

func newClient(base string) *client {
	return &client{base: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *client) libraries(ctx context.Context) ([]library, error) {
	var result []library
	return result, c.do(ctx, http.MethodGet, "/api/libraries", nil, http.StatusOK, &result)
}

func (c *client) deleteLibrary(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, "/api/libraries/"+strconv.FormatInt(id, 10), nil, http.StatusNoContent, nil)
}

func (c *client) createLibrary(ctx context.Context, name, path string) (library, error) {
	var result library
	input := map[string]string{"name": name, "path": path}
	return result, c.do(ctx, http.MethodPost, "/api/libraries", input, http.StatusCreated, &result)
}

func (c *client) requestScan(ctx context.Context, libraryID int64) error {
	input := map[string]int64{"library_id": libraryID}
	return c.do(ctx, http.MethodPost, "/api/scans", input, http.StatusAccepted, nil)
}

// waitForScan polls the scan status until the scan leaves its active phases.
func (c *client) waitForScan(ctx context.Context) (scanStatus, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		var status scanStatus
		if err := c.do(ctx, http.MethodGet, "/api/scans/status", nil, http.StatusOK, &status); err != nil {
			return status, fmt.Errorf("scan status: %w", err)
		}
		if status.Phase != "discovering" && status.Phase != "importing" {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return status, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *client) do(ctx context.Context, method, path string, input any, want int, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != want {
		var problem struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&problem)
		return fmt.Errorf("%s %s: %s %s", method, path, response.Status, problem.Error)
	}
	if output == nil {
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}
