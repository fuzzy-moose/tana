package httpapi

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorDownloads(client *collectorapi.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/collector/downloads", func(w http.ResponseWriter, r *http.Request) {
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
		result, err := client.ListDownloads(r.Context(), limit, offset)
		if err != nil {
			collectorDownloadError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("POST /api/collector/downloads", func(w http.ResponseWriter, r *http.Request) {
		var ref panda.GalleryRef
		if !server.DecodeJSON(w, r, &ref) {
			return
		}
		job, err := client.SubmitDownload(r.Context(), ref)
		if err != nil {
			collectorDownloadError(w, err)
			return
		}
		w.Header().Set("Location", "/api/collector/downloads/"+strconv.FormatInt(job.GalleryID, 10))
		server.WriteJSON(w, http.StatusAccepted, job)
	})
	jobOperation := func(operation func(context.Context, int64) (collectorapi.DownloadJob, error)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			job, err := operation(r.Context(), collectorDownloadID(r))
			if err != nil {
				collectorDownloadError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, job)
		}
	}
	mux.HandleFunc("GET /api/collector/downloads/{id}", jobOperation(client.GetDownload))
	mux.HandleFunc("POST /api/collector/downloads/{id}/retry", jobOperation(client.RetryDownload))
	mux.HandleFunc("POST /api/collector/downloads/{id}/cancel", jobOperation(client.CancelDownload))
	mux.HandleFunc("DELETE /api/collector/downloads/{id}", func(w http.ResponseWriter, r *http.Request) {
		if err := client.DeleteDownload(r.Context(), collectorDownloadID(r)); err != nil {
			collectorDownloadError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/collector/downloads/{id}/file", func(w http.ResponseWriter, r *http.Request) {
		id := collectorDownloadID(r)
		response, err := client.OpenDownload(r.Context(), id, r.Method, r.Header)
		if err != nil {
			collectorDownloadError(w, err)
			return
		}
		defer response.Body.Close()
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			collectorDownloadError(w, err)
			return
		}
		for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified"} {
			if value := response.Header.Get(name); value != "" {
				w.Header().Set(name, value)
			}
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": "[" + strconv.FormatInt(id, 10) + "].zip"}))
		w.Header().Set("Cache-Control", "private, no-store")
		w.WriteHeader(response.StatusCode)
		if _, err := io.Copy(w, response.Body); err != nil {
			server.GetHTTPContext(r).Err = err
			panic(http.ErrAbortHandler)
		}
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func collectorDownloadID(r *http.Request) int64 {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0
	}
	return id
}

func collectorDownloadError(w http.ResponseWriter, err error) {
	status, code := http.StatusBadGateway, "collector_unavailable"
	if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
		switch upstream.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "collector_unauthorized"
		case http.StatusBadRequest:
			status, code = http.StatusBadRequest, "invalid_gallery_reference"
		case http.StatusNotFound:
			status, code = http.StatusNotFound, "download_not_found"
		case http.StatusConflict:
			status, code = http.StatusConflict, "download_state_conflict"
			if upstream.Code == "gallery token conflicts with existing download" {
				code = "download_token_conflict"
			}
		}
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
