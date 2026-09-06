package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCreateLibrary(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		result, err := libraries.Create(r.Context(), input.Name, input.Path)
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		w.Header().Set("Location", "/api/libraries/"+strconv.FormatInt(result.ID, 10))
		server.WriteJSON(w, http.StatusCreated, result)
	})
}

func HandleListLibraries(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := libraries.List(r.Context())
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGetLibrary(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		result, err := libraries.Get(r.Context(), id)
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleRenameLibrary(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		var input struct {
			Name string `json:"name"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		result, err := libraries.Rename(r.Context(), id, input.Name)
		if err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleDeleteLibrary(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		if err := libraries.Delete(r.Context(), id); err != nil {
			writeLibraryError(w, r, err)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCheckLibraryAvailability(libraries *library.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		if err := libraries.RequestCheck(r.Context(), id); err != nil {
			writeLibraryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
	})
}

func writeLibraryError(w http.ResponseWriter, r *http.Request, err error) {
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
		server.GetHTTPContext(r).Err = err
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
