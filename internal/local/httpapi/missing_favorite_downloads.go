package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/favoritedownloads"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandlePreviewMissingFavoriteDownloads(service *favoritedownloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		category, ok := missingFavoriteCategory(w, r, service)
		if !ok {
			return
		}
		result, err := service.Preview(r.Context(), category)
		if err != nil {
			writeMissingFavoriteDownloadError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleSubmitMissingFavoriteDownloads(service *favoritedownloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		category, ok := missingFavoriteCategory(w, r, service)
		if !ok {
			return
		}
		var input struct {
			PlanID string `json:"plan_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		// Once confirmation is fully received, a browser disconnect must not
		// interrupt admission. The collector commits the whole batch atomically.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Minute)
		defer cancel()
		_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
		result, err := service.Execute(ctx, category, input.PlanID)
		if err != nil {
			writeMissingFavoriteDownloadError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusAccepted, result)
	})
}

func missingFavoriteCategory(w http.ResponseWriter, r *http.Request, service *favoritedownloads.Service) (int, bool) {
	if service == nil {
		server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
		return 0, false
	}
	category, err := strconv.Atoi(r.PathValue("category"))
	if err != nil || category < 0 || category > 9 {
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_category"})
		return 0, false
	}
	return category, true
}

func writeMissingFavoriteDownloadError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, favoritedownloads.ErrInvalidPreview) {
		server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "missing_download_preview_invalid"})
		return
	}
	if _, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
		collectorDownloadError(w, err)
		return
	}
	server.GetHTTPContext(r).Err = err
	server.WriteJSON(w, http.StatusBadGateway, map[string]string{"error": "collector_unavailable"})
}
