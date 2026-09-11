package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorSubmitReferenceImport(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		filename := r.URL.Query().Get("filename")
		if strings.TrimSpace(filename) == "" {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_filename"})
			return
		}
		if err := server.PrepareUpload(w, r, collectorapi.MaxReferenceImportBytes); err != nil {
			collectorReferenceImportError(w, err)
			return
		}
		result, err := client.SubmitReferenceImport(r.Context(), filename, r.Body)
		if err != nil {
			collectorReferenceImportError(w, err)
			return
		}
		w.Header().Set("Location", "/api/collector/reference-imports/"+url.PathEscape(result.ID))
		server.WriteJSON(w, http.StatusAccepted, result)
	})
}

func HandleCollectorListReferenceImports(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
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
		result, err := client.ListReferenceImports(r.Context(), limit, offset)
		if err != nil {
			collectorReferenceImportError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCollectorGetReferenceImport(client *collectorapi.Client) http.Handler {
	return handleCollectorReferenceImport(client, client.GetReferenceImport)
}

func HandleCollectorCancelReferenceImport(client *collectorapi.Client) http.Handler {
	return handleCollectorReferenceImport(client, client.CancelReferenceImport)
}

func HandleCollectorRetryReferenceImport(client *collectorapi.Client) http.Handler {
	return handleCollectorReferenceImport(client, client.RetryReferenceImport)
}

func handleCollectorReferenceImport(client *collectorapi.Client, operation func(context.Context, string) (collectorapi.ReferenceImport, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		result, err := operation(r.Context(), r.PathValue("id"))
		if err != nil {
			collectorReferenceImportError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func collectorReferenceImportError(w http.ResponseWriter, err error) {
	status, code := http.StatusBadGateway, "collector_unavailable"
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge {
		status, code = http.StatusRequestEntityTooLarge, "import_too_large"
	} else if errors.Is(err, server.ErrUploadInterrupted) || errors.Is(err, context.Canceled) {
		status, code = http.StatusBadRequest, "invalid_upload"
	} else if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
		switch upstream.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "collector_unauthorized"
		case http.StatusBadRequest:
			status, code = http.StatusBadRequest, "invalid_upload"
		case http.StatusNotFound:
			status, code = http.StatusNotFound, "import_not_found"
		case http.StatusConflict:
			status, code = http.StatusConflict, "import_state_conflict"
		case http.StatusRequestEntityTooLarge:
			status, code = http.StatusRequestEntityTooLarge, "import_too_large"
		}
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
