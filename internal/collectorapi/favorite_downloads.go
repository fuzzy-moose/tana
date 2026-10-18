package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

var ErrInvalidDownloadCategories = errors.New("invalid_download_categories")

type FavoriteDownloadSettings struct {
	Categories []int `json:"categories"`
}

func (s FavoriteDownloadSettings) Validate() error {
	if s.Categories == nil || len(s.Categories) > 10 {
		return ErrInvalidDownloadCategories
	}
	var seen [10]bool
	for _, category := range s.Categories {
		if category < 0 || category > 9 || seen[category] {
			return ErrInvalidDownloadCategories
		}
		seen[category] = true
	}
	return nil
}

func (c *Client) FavoriteDownloadSettings(ctx context.Context) (FavoriteDownloadSettings, error) {
	return c.favoriteDownloadSettings(ctx, http.MethodGet, nil)
}

func (c *Client) SetFavoriteDownloadSettings(ctx context.Context, settings FavoriteDownloadSettings) (FavoriteDownloadSettings, error) {
	if err := settings.Validate(); err != nil {
		return FavoriteDownloadSettings{}, err
	}
	body, err := json.Marshal(settings)
	if err != nil {
		return FavoriteDownloadSettings{}, err
	}
	return c.favoriteDownloadSettings(ctx, http.MethodPut, bytes.NewReader(body))
}

func (c *Client) favoriteDownloadSettings(ctx context.Context, method string, body io.Reader) (FavoriteDownloadSettings, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var settings FavoriteDownloadSettings
	response, err := c.controlRequest(ctx, method, "/api/favorites/download-settings", body)
	if err != nil {
		return settings, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return settings, &HTTPError{StatusCode: response.StatusCode}
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&settings); err != nil {
		return settings, err
	}
	return settings, settings.Validate()
}
