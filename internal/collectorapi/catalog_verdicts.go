package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// The two verdicts a user can give a Panda gallery.
const (
	VerdictInteresting = "interesting"
	VerdictNotForMe    = "not_for_me"
)

func ValidVerdict(verdict string) bool {
	return verdict == VerdictInteresting || verdict == VerdictNotForMe
}

// SetCatalogVerdict gives the gallery the verdict, replacing any other one.
func (c *Client) SetCatalogVerdict(ctx context.Context, galleryID int64, verdict string) error {
	body, err := json.Marshal(struct {
		Verdict string `json:"verdict"`
	}{verdict})
	if err != nil {
		return err
	}
	return c.catalogVerdictRequest(ctx, http.MethodPut, galleryID, bytes.NewReader(body))
}

// RemoveCatalogVerdict removes the gallery's verdict, if it has one.
func (c *Client) RemoveCatalogVerdict(ctx context.Context, galleryID int64) error {
	return c.catalogVerdictRequest(ctx, http.MethodDelete, galleryID, nil)
}

func (c *Client) catalogVerdictRequest(ctx context.Context, method string, galleryID int64, body io.Reader) error {
	response, err := c.controlRequest(ctx, method, fmt.Sprintf("/api/catalog/verdicts/%d", galleryID), body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return downloadResponseError(response)
	}
	return nil
}
