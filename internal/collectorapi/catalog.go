package collectorapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/gallerysearch"
)

type CatalogOptions struct {
	Query           string
	Page            int
	PageSize        int
	IncludeExpunged bool
}

type CatalogItem struct {
	GalleryID    int64     `json:"gallery_id"`
	Title        string    `json:"title"`
	ThumbnailURL string    `json:"thumbnail_url"`
	PageCount    int64     `json:"page_count"`
	PostedAt     time.Time `json:"posted_at"`
	URL          string    `json:"url"`
}

type CatalogResult struct {
	Items      []CatalogItem `json:"items"`
	Total      int64         `json:"total"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	TotalPages int           `json:"total_pages"`
}

type CatalogCompletion = gallerysearch.Completion

func (c *Client) Catalog(ctx context.Context, options CatalogOptions) (CatalogResult, error) {
	params := url.Values{
		"q":                {options.Query},
		"page":             {strconv.Itoa(options.Page)},
		"page_size":        {strconv.Itoa(options.PageSize)},
		"include_expunged": {strconv.FormatBool(options.IncludeExpunged)},
	}
	var result CatalogResult
	err := c.catalogRequest(ctx, "/api/catalog?"+params.Encode(), &result)
	if err == nil && (result.Items == nil || result.Total < 0 || result.Page < 1 || result.PageSize < 1 || result.PageSize > 100 || result.TotalPages < 1) {
		err = fmt.Errorf("collector returned an invalid catalog page")
	}
	return result, err
}

func (c *Client) CompleteCatalog(ctx context.Context, query string, cursor int) (CatalogCompletion, error) {
	params := url.Values{"q": {query}, "cursor": {strconv.Itoa(cursor)}}
	var result CatalogCompletion
	err := c.catalogRequest(ctx, "/api/catalog/completions?"+params.Encode(), &result)
	if err == nil && result.Items == nil {
		err = fmt.Errorf("collector returned invalid catalog completions")
	}
	return result, err
}

func (c *Client) catalogRequest(ctx context.Context, path string, result any) error {
	response, err := c.controlRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return downloadResponseError(response)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result)
}
