package httpapi

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleLookupMetadata(service *metadata.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		result, err := service.Lookup(r.Context(), input.GalleryIDs)
		if err != nil {
			metadataError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleRequestFetch(service *metadata.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Galleries []panda.GalleryRef `json:"galleries"`
		}
		if !server.DecodeJSONLimit(w, r, &input, 1024*1024) {
			return
		}
		job, err := service.RequestFetch(r.Context(), input.Galleries)
		if err != nil {
			metadataError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/metadata/fetches/"+job.ID)
		server.WriteJSON(w, http.StatusAccepted, job)
	})
}

func HandleGetFetchJob(service *metadata.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		job, err := service.GetFetchJob(r.Context(), r.PathValue("id"))
		if err != nil {
			metadataError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, job)
	})
}

func metadataError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, metadata.ErrInvalidBatch):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_batch", "message": err.Error()})
	case errors.Is(err, sql.ErrNoRows):
		server.NotFound(w, r)
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
