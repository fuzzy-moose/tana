package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorFavoriteDownloadSettings(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var settings collectorapi.FavoriteDownloadSettings
		var err error
		if r.Method == http.MethodPut {
			if !server.DecodeJSON(w, r, &settings) {
				return
			}
			settings, err = client.SetFavoriteDownloadSettings(r.Context(), settings)
		} else {
			settings, err = client.FavoriteDownloadSettings(r.Context())
		}
		if err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			if errors.Is(err, collectorapi.ErrInvalidDownloadCategories) {
				status, code = http.StatusBadRequest, "invalid_download_categories"
			} else if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok &&
				(upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
				code = "collector_unauthorized"
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, settings)
	})
}
