package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

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
