package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/refresh"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandlePreviewLibraryRefresh(service *refresh.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var libraryID int64
		if r.PathValue("id") != "" {
			var ok bool
			libraryID, ok = pathID(w, r)
			if !ok {
				return
			}
		}
		// Multiple storage checks can outlast the normal response deadline.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		result, err := service.Preview(r.Context(), libraryID)
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleExecuteLibraryRefresh(service *refresh.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PlanID    string  `json:"plan_id"`
			SourceIDs []int64 `json:"source_ids"`
		}
		if !server.DecodeJSONLimit(w, r, &input, 8<<20) {
			return
		}
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		result, err := service.Execute(r.Context(), input.PlanID, input.SourceIDs)
		if errors.Is(err, refresh.ErrInvalidSelection) {
			server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "refresh_preview_invalid"})
			return
		}
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
