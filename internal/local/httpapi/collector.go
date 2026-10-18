package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorFavoritesStatus(client *collectorapi.Client) http.Handler {
	return handleCollectorStatusView(client, client.FavoritesStatus)
}

func HandleCollectorInventoryStatus(client *collectorapi.Client) http.Handler {
	return handleCollectorStatusView(client, client.InventoryStatus)
}

func HandleCollectorMetadataStatus(client *collectorapi.Client) http.Handler {
	return handleCollectorStatusView(client, client.MetadataStatus)
}

func HandleCollectorSummary(client *collectorapi.Client) http.Handler {
	return handleCollectorStatusView(client, client.Summary)
}

func HandleCollectorMetadataCollectionPause(client *collectorapi.Client, paused bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var input struct{}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.SetMainBackgroundPaused(r.Context(), paused); err != nil {
			collectorControlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorResetBan(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		group := r.PathValue("group")
		if group != "main" && group != "authenticated" && group != "proxies" {
			server.NotFound(w, r)
			return
		}
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var input struct{}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.ResetBan(r.Context(), group); err != nil {
			collectorControlError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func collectorControlError(w http.ResponseWriter, err error) {
	code := "collector_unavailable"
	if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok &&
		(upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
		code = "collector_unauthorized"
	}
	server.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": code})
}

func handleCollectorStatusView[T any](client *collectorapi.Client, read func(context.Context) (T, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		result, err := read(r.Context())
		if err != nil {
			code := "collector_unavailable"
			if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok &&
				(upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
				code = "collector_unauthorized"
			}
			server.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCollectorStatus(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusOK, collectorapi.ConnectionStatus{CheckedAt: time.Now().UTC()})
			return
		}
		server.WriteJSON(w, http.StatusOK, client.Status(r.Context()))
	})
}

func HandleCollectorSyncFavorites(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var input struct {
			Full bool `json:"full"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.SyncFavorites(r.Context(), r.PathValue("category"), input.Full); err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			if errors.Is(err, collectorapi.ErrInvalidCategory) {
				status, code = http.StatusBadRequest, "invalid_category"
			} else if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok &&
				(upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
				code = "collector_unauthorized"
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
}

func HandleCollectorMetadataSettings(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var input struct {
			FeedMetadataDelayHours *int64 `json:"feed_metadata_delay_hours"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if input.FeedMetadataDelayHours == nil {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_metadata_settings"})
			return
		}
		result, err := client.SetMetadataSettings(r.Context(), collectorapi.MetadataSettings{FeedMetadataDelayHours: *input.FeedMetadataDelayHours})
		if err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			if errors.Is(err, collectorapi.ErrInvalidMetadataSettings) {
				status, code = http.StatusBadRequest, "invalid_metadata_settings"
			} else if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
				if upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden {
					code = "collector_unauthorized"
				}
				if upstream.StatusCode == http.StatusBadRequest {
					status, code = http.StatusBadRequest, "invalid_metadata_settings"
				}
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
