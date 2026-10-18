package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/delivery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleLibraryDeliveries(service *delivery.Service, client *collectorapi.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/library-deliveries", func(w http.ResponseWriter, r *http.Request) {
		batches, err := service.List(r.Context())
		if err != nil {
			writeDeliveryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, map[string]any{"batches": batches})
	})
	mux.HandleFunc("POST /api/library-deliveries", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			LibraryID int64 `json:"library_id"`
			GalleryID int64 `json:"gallery_id"`
			All       bool  `json:"all"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if input.LibraryID <= 0 || (input.All && input.GalleryID != 0) || (!input.All && input.GalleryID <= 0) {
			writeDeliveryError(w, r, delivery.ErrInvalid)
			return
		}
		var ids []int64
		if input.All {
			var err error
			ids, err = client.CompletedDownloadIDs(r.Context())
			if err != nil {
				collectorDownloadError(w, err)
				return
			}
		} else {
			job, err := client.GetDownload(r.Context(), input.GalleryID)
			if err != nil {
				collectorDownloadError(w, err)
				return
			}
			if job.GalleryID != input.GalleryID || job.State != "completed" {
				server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "download_state_conflict"})
				return
			}
			ids = []int64{input.GalleryID}
		}
		// Once the complete selection is known, admission belongs to Tana even
		// if the browser disconnects before receiving the accepted batch.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
		defer cancel()
		batch, err := service.Start(ctx, input.LibraryID, ids)
		if err != nil {
			writeDeliveryError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/library-deliveries/"+strconv.FormatInt(batch.ID, 10))
		server.WriteJSON(w, http.StatusAccepted, batch)
	})
	operation := func(fn func(context.Context, int64) (delivery.Batch, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
			if err != nil || id <= 0 {
				writeDeliveryError(w, r, delivery.ErrInvalid)
				return
			}
			batch, err := fn(r.Context(), id)
			if err != nil {
				writeDeliveryError(w, r, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, batch)
		}
	}
	mux.HandleFunc("GET /api/library-deliveries/{id}", operation(service.Get))
	mux.HandleFunc("POST /api/library-deliveries/{id}/resume", operation(service.Resume))
	mux.HandleFunc("POST /api/library-deliveries/{id}/stop", operation(service.Stop))
	mux.HandleFunc("POST /api/library-deliveries/{id}/retry", operation(service.Retry))
	mux.HandleFunc("POST /api/library-deliveries/{id}/retry-cleanup", operation(service.RetryCleanup))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil || client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func writeDeliveryError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, delivery.ErrInvalid):
		status, code = http.StatusBadRequest, "invalid_delivery"
	case errors.Is(err, delivery.ErrActive):
		status, code = http.StatusConflict, "delivery_active"
	case errors.Is(err, delivery.ErrNotFound):
		status, code = http.StatusNotFound, "delivery_not_found"
	case errors.Is(err, library.ErrNotFound):
		status, code = http.StatusNotFound, "library_not_found"
	case errors.Is(err, delivery.ErrUnavailable):
		status, code = http.StatusConflict, "library_unavailable"
	default:
		server.GetHTTPContext(r).Err = err
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
