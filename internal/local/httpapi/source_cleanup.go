package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/fuzzy-moose/tana/internal/local/cleanup"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandlePreviewSourceCleanup(service *cleanup.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		// Global presence checks can outlast the normal response deadline on a NAS.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		result, err := service.Preview(r.Context())
		if err != nil {
			writeSourceCleanupError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleExecuteSourceCleanup(service *cleanup.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			PlanID    string  `json:"plan_id"`
			SourceIDs []int64 `json:"source_ids"`
		}
		if !server.DecodeJSONLimit(w, r, &input, 8<<20) {
			return
		}
		if service == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		// Keep the result deliverable after a large batch of filesystem removals.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		result, err := service.Execute(r.Context(), input.PlanID, input.SourceIDs)
		if err != nil {
			writeSourceCleanupError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func writeSourceCleanupError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, cleanup.ErrInvalidSelection) {
		server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "cleanup_preview_invalid"})
		return
	}
	if errors.Is(err, cleanup.ErrUnavailable) {
		server.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "collector_unavailable"})
		return
	}
	writeLibraryError(w, r, err)
}
