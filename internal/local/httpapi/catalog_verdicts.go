package httpapi

import (
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorSetCatalogVerdict(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		galleryID, ok := verdictGalleryID(w, r)
		if !ok {
			return
		}
		var input struct {
			Verdict string `json:"verdict"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if !collectorapi.ValidVerdict(input.Verdict) {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_verdict"})
			return
		}
		if err := client.SetCatalogVerdict(r.Context(), galleryID, input.Verdict); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorRemoveCatalogVerdict(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		galleryID, ok := verdictGalleryID(w, r)
		if !ok {
			return
		}
		if err := client.RemoveCatalogVerdict(r.Context(), galleryID); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// verdictGalleryID answers an ID that is not a positive integer with 400, not
// the 404 of pathID: the gallery reference itself is invalid.
func verdictGalleryID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	galleryID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || galleryID <= 0 {
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gallery_reference"})
		return 0, false
	}
	return galleryID, true
}
