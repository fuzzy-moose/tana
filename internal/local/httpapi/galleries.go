package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleListGalleries(galleries *gallery.SQLiteRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := positiveQuery(r, "page", 1)
		pageSize, sizeOK := positiveQuery(r, "page_size", 24)
		if !ok || !sizeOK || pageSize > 100 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			return
		}
		result, err := galleries.Browse(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), page, pageSize)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func positiveQuery(r *http.Request, key string, fallback int64) (int64, bool) {
	if !r.URL.Query().Has(key) {
		return fallback, true
	}
	n, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return n, err == nil && n > 0
}

func HandleCompleteGallerySearch(galleries *gallery.SQLiteRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor, err := strconv.Atoi(r.URL.Query().Get("cursor"))
		if err != nil {
			writeGalleryError(w, r, gallery.ErrInvalidQuery)
			return
		}
		result, err := galleries.Complete(r.Context(), r.URL.Query().Get("q"), cursor)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGetGallery(galleries *gallery.SQLiteRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := galleries.Detail(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGalleryImage(galleries *gallery.SQLiteRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
		if err != nil || number < 1 {
			server.NotFound(w, r)
			return
		}
		content, err := galleries.OpenImage(r.Context(), r.PathValue("id"), number)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		defer content.Close()
		// Removing source files can renumber gallery occurrences at this URL.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", content.ContentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.FormatInt(content.Size, 10))
		if r.Method != http.MethodHead {
			if _, err := io.Copy(w, content); err != nil {
				server.GetHTTPContext(r).Err = err
			}
		}
	})
}

func writeGalleryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, gallery.ErrInvalidQuery):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	case errors.Is(err, gallery.ErrNotFound):
		server.NotFound(w, r)
	case errors.Is(err, gallery.ErrImageUnavailable):
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "image_unavailable"})
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
