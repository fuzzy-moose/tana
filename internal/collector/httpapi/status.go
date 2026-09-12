package httpapi

import (
	"context"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.WriteJSON(w, http.StatusOK, collectorapi.Status{Available: true})
	})
}

func HandleFavoritesStatus(service *status.Service) http.Handler {
	return handleStatusView(service.Favorites, "favorites_status_unavailable")
}

func HandleInventoryStatus(service *status.Service) http.Handler {
	return handleStatusView(service.Inventory, "inventory_status_unavailable")
}

func HandleMetadataStatus(service *status.Service) http.Handler {
	return handleStatusView(service.Metadata, "metadata_status_unavailable")
}

func handleStatusView[T any](read func(context.Context) (T, error), code string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := read(r.Context())
		if err != nil {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
