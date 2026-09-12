package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"mime"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/downloads"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleSubmitDownload(service *downloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ref panda.GalleryRef
		if !server.DecodeJSON(w, r, &ref) {
			return
		}
		job, err := service.Submit(r.Context(), ref)
		if err != nil {
			downloadError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/downloads/"+strconv.FormatInt(job.GalleryID, 10))
		server.WriteJSON(w, http.StatusAccepted, job)
	})
}

func HandleListDownloads(service *downloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, offset := int64(100), int64(0)
		var err error
		if value := r.URL.Query().Get("limit"); value != "" {
			limit, err = strconv.ParseInt(value, 10, 64)
		}
		if err == nil {
			if value := r.URL.Query().Get("offset"); value != "" {
				offset, err = strconv.ParseInt(value, 10, 64)
			}
		}
		if err != nil || limit < 1 || limit > 100 || offset < 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			return
		}
		result, err := service.List(r.Context(), r.URL.Query().Get("state"), limit, offset)
		if err != nil {
			downloadError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGetDownload(service *downloads.Service) http.Handler {
	return handleDownloadJob(service.Get)
}

func HandleRetryDownload(service *downloads.Service) http.Handler {
	return handleDownloadJob(service.Retry)
}

func HandleCancelDownload(service *downloads.Service) http.Handler {
	return handleDownloadJob(service.Cancel)
}

func handleDownloadJob(operation func(context.Context, int64) (downloads.Job, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			downloadError(w, r, downloads.ErrInvalidReference)
			return
		}
		job, err := operation(r.Context(), id)
		if err != nil {
			downloadError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, job)
	})
}

func HandleDeleteDownload(service *downloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			downloadError(w, r, downloads.ErrInvalidReference)
			return
		}
		if err := service.Delete(r.Context(), id); err != nil {
			downloadError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleGetDownloadFile(service *downloads.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			downloadError(w, r, downloads.ErrInvalidReference)
			return
		}
		file, err := service.Open(r.Context(), id)
		if err != nil {
			downloadError(w, r, err)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			downloadError(w, r, err)
			return
		}
		// Retained archives can take longer to deliver than the API write timeout.
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			downloadError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": info.Name()}))
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}

func downloadError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, downloads.ErrInvalidState):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_state"})
	case errors.Is(err, downloads.ErrInvalidReference):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gallery_reference"})
	case errors.Is(err, downloads.ErrState), errors.Is(err, downloads.ErrTokenConflict):
		server.WriteJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, sql.ErrNoRows), errors.Is(err, os.ErrNotExist):
		server.NotFound(w, r)
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
