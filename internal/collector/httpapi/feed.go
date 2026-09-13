package httpapi

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleListFeedCaptures(service *feed.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit, offset := int64(25), int64(0)
		for _, parameter := range []struct {
			name string
			dest *int64
		}{{"limit", &limit}, {"offset", &offset}} {
			if r.URL.Query().Has(parameter.name) {
				value, err := strconv.ParseInt(r.URL.Query().Get(parameter.name), 10, 64)
				if err != nil {
					server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
					return
				}
				*parameter.dest = value
			}
		}
		failedOnly := false
		if r.URL.Query().Has("failed_only") {
			var err error
			failedOnly, err = strconv.ParseBool(r.URL.Query().Get("failed_only"))
			if err != nil {
				server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
				return
			}
		}
		result, err := service.ListCaptures(r.Context(), failedOnly, limit, offset)
		if err != nil {
			if errors.Is(err, feed.ErrInvalidPagination) {
				server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			} else {
				server.GetHTTPContext(r).Err = err
				server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "feed_unavailable"})
			}
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleFeedCaptureFile(service *feed.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			server.NotFound(w, r)
			return
		}
		body, err := service.CaptureBody(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			server.NotFound(w, r)
			return
		}
		if err != nil {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "feed_unavailable"})
			return
		}
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "feed_unavailable"})
			return
		}
		filename := fmt.Sprintf("panda-feed-%d.xml", id)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		http.ServeContent(w, r, filename, time.Time{}, bytes.NewReader(body))
	})
}

func HandleFeed(service *feed.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result collectorapi.FeedStatus
		var err error
		status := http.StatusOK
		if r.Method == http.MethodPost {
			var input struct{}
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			result, err = service.Refresh(r.Context())
			status = http.StatusAccepted
		} else {
			result, err = service.Status(r.Context())
		}
		if err != nil {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "feed_unavailable"})
			return
		}
		server.WriteJSON(w, status, result)
	})
}
