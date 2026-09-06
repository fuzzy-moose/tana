package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type Client struct {
	endpoint string
	token    string
	http     *http.Client
}

func NewClient(baseURL, token string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("collector URL must be an absolute HTTP(S) URL without credentials, query or fragment")
	}
	if token == "" || strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return nil, fmt.Errorf("collector API token must be nonempty and contain no whitespace")
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/metadata/lookup"
	u.RawPath = ""
	return &Client{endpoint: u.String(), token: token, http: &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) Lookup(ctx context.Context, ids []int64) (LookupResult, error) {
	body, err := json.Marshal(struct {
		GalleryIDs []int64 `json:"gallery_ids"`
	}{ids})
	if err != nil {
		return LookupResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return LookupResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.http.Do(req)
	if err != nil {
		return LookupResult{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return LookupResult{}, fmt.Errorf("collector lookup returned HTTP %d", response.StatusCode)
	}
	var result LookupResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&result); err != nil {
		return LookupResult{}, fmt.Errorf("decode collector lookup: %w", err)
	}
	// An incomplete or contradictory response must never finish durable work.
	wanted := make(map[int64]bool, len(ids))
	for _, id := range ids {
		wanted[id] = true
	}
	consume := func(id int64) error {
		if !wanted[id] {
			return fmt.Errorf("collector lookup returned unexpected or duplicate ID %d", id)
		}
		delete(wanted, id)
		return nil
	}
	for _, gallery := range result.Galleries {
		if gallery.Metadata.Error != "" {
			return LookupResult{}, fmt.Errorf("collector returned failed metadata as available")
		}
		if err := consume(gallery.Metadata.ID); err != nil {
			return LookupResult{}, err
		}
	}
	for _, group := range [][]int64{result.PendingIDs, result.FailedIDs, result.UnknownIDs} {
		for _, id := range group {
			if err := consume(id); err != nil {
				return LookupResult{}, err
			}
		}
	}
	if len(wanted) != 0 {
		return LookupResult{}, fmt.Errorf("collector lookup omitted %d IDs", len(wanted))
	}
	return result, nil
}
