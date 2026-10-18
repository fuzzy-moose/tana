package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCatalogFacts(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		result, err := service.Facts(r.Context(), input.GalleryIDs)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
