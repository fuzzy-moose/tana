package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleFavoriteDownloadSettings(service *favorites.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			var settings collectorapi.FavoriteDownloadSettings
			if !server.DecodeJSON(w, r, &settings) {
				return
			}
			if err := service.SetDownloadSettings(r.Context(), settings); err != nil {
				status := http.StatusServiceUnavailable
				if errors.Is(err, collectorapi.ErrInvalidDownloadCategories) {
					status = http.StatusBadRequest
				}
				server.WriteJSON(w, status, map[string]string{"error": err.Error()})
				return
			}
		}
		settings, err := service.DownloadSettings(r.Context())
		if err != nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "download_settings_unavailable"})
			return
		}
		server.WriteJSON(w, http.StatusOK, settings)
	})
}
