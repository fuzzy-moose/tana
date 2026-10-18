package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

func (c *Client) StartSitemap(ctx context.Context, force bool) (SitemapStatus, error) {
	body, err := json.Marshal(struct {
		Force bool `json:"force"`
	}{force})
	if err != nil {
		return SitemapStatus{}, err
	}
	return c.sitemapRequest(ctx, http.MethodPost, "start", body)
}

func (c *Client) CancelSitemap(ctx context.Context) (SitemapStatus, error) {
	return c.sitemapRequest(ctx, http.MethodPost, "cancel", []byte(`{}`))
}

func (c *Client) RetrySitemap(ctx context.Context) (SitemapStatus, error) {
	return c.sitemapRequest(ctx, http.MethodPost, "retry", []byte(`{}`))
}

func (c *Client) SitemapStatus(ctx context.Context) (SitemapStatus, error) {
	return c.sitemapRequest(ctx, http.MethodGet, "status", nil)
}

func (c *Client) sitemapRequest(ctx context.Context, method, action string, body []byte) (SitemapStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var result SitemapStatus
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	response, err := c.controlRequest(ctx, method, "/api/sitemap/"+action, reader)
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
