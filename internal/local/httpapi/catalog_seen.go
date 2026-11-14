package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorRecordCatalogDisplays(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			GalleryIDs  []int64 `json:"gallery_ids"`
			ScoreFilter string  `json:"score_filter"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		valid := len(input.GalleryIDs) > 0 && len(input.GalleryIDs) <= collectorapi.MaxCatalogDisplays &&
			collectorapi.ValidScoreFilter(input.ScoreFilter)
		for _, id := range input.GalleryIDs {
			valid = valid && id > 0
		}
		if !valid {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_batch"})
			return
		}
		if err := client.RecordCatalogDisplays(r.Context(), input.GalleryIDs, input.ScoreFilter); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorCatalogCatchUp(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			GalleryID int64 `json:"gallery_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if input.GalleryID <= 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gallery_reference"})
			return
		}
		if err := client.CatalogCatchUp(r.Context(), input.GalleryID); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
