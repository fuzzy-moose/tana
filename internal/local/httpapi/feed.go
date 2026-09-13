package httpapi

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorFeedCaptures(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
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
		if limit < 1 || limit > 100 || offset < 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			return
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
		result, err := client.ListFeedCaptures(r.Context(), failedOnly, limit, offset)
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCollectorFeedCaptureFile(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		response, err := client.OpenFeedCapture(r.Context(), id, r.Method)
		if err != nil {
			if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok && upstream.StatusCode == http.StatusNotFound {
				server.NotFound(w, r)
			} else {
				writeCatalogCollectorError(w, err)
			}
			return
		}
		defer response.Body.Close()
		if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
			writeCatalogCollectorError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="panda-feed-%d.xml"`, id))
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if length := response.Header.Get("Content-Length"); length != "" {
			w.Header().Set("Content-Length", length)
		}
		w.WriteHeader(http.StatusOK)
		if _, err := io.Copy(w, response.Body); err != nil {
			server.GetHTTPContext(r).Err = err
			panic(http.ErrAbortHandler)
		}
	})
}
