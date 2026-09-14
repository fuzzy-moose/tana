package collectorapi

import (
	"context"
	"net/http"
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
