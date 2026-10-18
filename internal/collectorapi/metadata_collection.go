package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) SetMainBackgroundPaused(ctx context.Context, paused bool) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	action := "resume"
	if paused {
		action = "pause"
	}
	response, err := c.controlRequest(ctx, http.MethodPost, "/api/metadata/"+action, strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return nil
}

// ResetBan lifts the "main", "authenticated", or "proxies" request group's ban.
func (c *Client) ResetBan(ctx context.Context, group string) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	response, err := c.controlRequest(ctx, http.MethodPost, "/api/bans/"+url.PathEscape(group)+"/reset", strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return &HTTPError{StatusCode: response.StatusCode}
	}
	return nil
}

const MaxFeedMetadataDelayHours int64 = 8760

var ErrInvalidMetadataSettings = errors.New("feed metadata delay must be an integer from 0 to 8760 hours")

type MetadataSettings struct {
	FeedMetadataDelayHours int64 `json:"feed_metadata_delay_hours"`
}

func (s MetadataSettings) Validate() error {
	if s.FeedMetadataDelayHours < 0 || s.FeedMetadataDelayHours > MaxFeedMetadataDelayHours {
		return ErrInvalidMetadataSettings
	}
	return nil
}

func (c *Client) SetMetadataSettings(ctx context.Context, settings MetadataSettings) (MetadataSettings, error) {
	if err := settings.Validate(); err != nil {
		return MetadataSettings{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	body, err := json.Marshal(settings)
	if err != nil {
		return MetadataSettings{}, err
	}
	response, err := c.controlRequest(ctx, http.MethodPut, "/api/metadata/settings", bytes.NewReader(body))
	if err != nil {
		return MetadataSettings{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return MetadataSettings{}, &HTTPError{StatusCode: response.StatusCode}
	}
	var result MetadataSettings
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return result, err
	}
	return result, result.Validate()
}
