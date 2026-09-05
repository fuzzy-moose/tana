package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local/library"
)

type libraryAPI struct {
	libraries *library.Service
	logger    *slog.Logger
}

func (a libraryAPI) create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := a.libraries.Create(r.Context(), input.Name, input.Path)
	if err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("Location", "/api/libraries/"+result.ID)
	writeJSON(w, http.StatusCreated, result)
}

func (a libraryAPI) list(w http.ResponseWriter, r *http.Request) {
	result, err := a.libraries.List(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a libraryAPI) get(w http.ResponseWriter, r *http.Request) {
	result, err := a.libraries.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a libraryAPI) rename(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := a.libraries.Rename(r.Context(), r.PathValue("id"), input.Name)
	if err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a libraryAPI) delete(w http.ResponseWriter, r *http.Request) {
	if err := a.libraries.Delete(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (a libraryAPI) check(w http.ResponseWriter, r *http.Request) {
	if err := a.libraries.RequestCheck(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (a libraryAPI) fail(w http.ResponseWriter, err error) {
	status, code := http.StatusInternalServerError, "internal_error"
	switch {
	case errors.Is(err, library.ErrInvalidName):
		status, code = http.StatusBadRequest, "invalid_name"
	case errors.Is(err, library.ErrInvalidPath):
		status, code = http.StatusBadRequest, "invalid_path"
	case errors.Is(err, library.ErrRootUnavailable):
		status, code = http.StatusUnprocessableEntity, "root_unavailable"
	case errors.Is(err, library.ErrRootConflict):
		status, code = http.StatusConflict, "root_conflict"
	case errors.Is(err, library.ErrStorageOverlap):
		status, code = http.StatusConflict, "storage_overlap"
	case errors.Is(err, library.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, context.DeadlineExceeded):
		status, code = http.StatusServiceUnavailable, "operation_timeout"
	case errors.Is(err, context.Canceled):
		status, code = http.StatusRequestTimeout, "request_canceled"
	default:
		a.logger.Error("library_request_failed", "error", err)
	}
	writeJSON(w, status, map[string]string{"error": code})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	err := d.Decode(target)
	if err == nil {
		var extra any
		if d.Decode(&extra) == io.EOF {
			return true
		}
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
