package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCompletedDownloads(service *downloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids, err := service.CompletedDownloadIDs(r.Context())
		if err != nil {
			downloadError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}{ids})
	})
}
