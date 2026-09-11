package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleAcceptReferenceImport(service *metadata.ReferenceImports) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filename := r.URL.Query().Get("filename")
		if strings.TrimSpace(filename) == "" {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_filename"})
			return
		}
		if err := server.PrepareUpload(w, r, collectorapi.MaxReferenceImportBytes); err != nil {
			referenceImportError(w, r, err)
			return
		}
		result, err := service.Accept(r.Context(), filename, r.Body)
		if err != nil {
			referenceImportError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/reference-imports/"+url.PathEscape(result.ID))
		server.WriteJSON(w, http.StatusAccepted, result)
	})
}

func HandleListReferenceImports(service *metadata.ReferenceImports) http.Handler {
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
		imports, err := service.List(r.Context(), limit, offset)
		if err != nil {
			referenceImportError(w, r, err)
			return
		}
		if imports == nil {
			imports = []collectorapi.ReferenceImport{}
		}
		server.WriteJSON(w, http.StatusOK, collectorapi.ReferenceImportList{Imports: imports})
	})
}

func HandleGetReferenceImport(service *metadata.ReferenceImports) http.Handler {
	return handleReferenceImport(service.Get)
}

func HandleCancelReferenceImport(service *metadata.ReferenceImports) http.Handler {
	return handleReferenceImport(service.Cancel)
}

func HandleRetryReferenceImport(service *metadata.ReferenceImports) http.Handler {
	return handleReferenceImport(service.Retry)
}

func handleReferenceImport(operation func(context.Context, string) (collectorapi.ReferenceImport, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := operation(r.Context(), r.PathValue("id"))
		if err != nil {
			referenceImportError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func referenceImportError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge || errors.Is(err, metadata.ErrImportTooLarge) {
		status, code = http.StatusRequestEntityTooLarge, "import_too_large"
	} else {
		switch {
		case errors.Is(err, server.ErrUploadInterrupted), errors.Is(err, context.Canceled):
			status, code = http.StatusBadRequest, "invalid_upload"
		case errors.Is(err, metadata.ErrImportState):
			status, code = http.StatusConflict, "import_state_conflict"
		case errors.Is(err, sql.ErrNoRows):
			status, code = http.StatusNotFound, "import_not_found"
		default:
			server.GetHTTPContext(r).Err = err
		}
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
