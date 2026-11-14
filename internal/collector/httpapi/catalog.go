package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCatalog(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		options := collectorapi.CatalogOptions{
			Query: r.URL.Query().Get("q"), Categories: r.URL.Query()["category"],
			DefaultQuery: r.URL.Query().Get("default_q"), DefaultCategories: r.URL.Query()["default_category"],
			Cursor: r.URL.Query().Get("cursor"), PageSize: 24, Verdict: r.URL.Query().Get("verdict"),
		}
		if r.URL.Query().Has("page_size") {
			value, err := strconv.Atoi(r.URL.Query().Get("page_size"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidPagination)
				return
			}
			options.PageSize = value
		}
		if r.URL.Query().Has("include_expunged") {
			var err error
			options.IncludeExpunged, err = strconv.ParseBool(r.URL.Query().Get("include_expunged"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidQuery)
				return
			}
		}
		if r.URL.Query().Has("unseen") {
			var err error
			options.Unseen, err = strconv.ParseBool(r.URL.Query().Get("unseen"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidQuery)
				return
			}
		}
		result, err := service.List(r.Context(), options)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCompleteCatalog(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor, err := strconv.Atoi(r.URL.Query().Get("cursor"))
		if err != nil {
			catalogError(w, r, catalog.ErrInvalidQuery)
			return
		}
		result, err := service.Complete(r.Context(), r.URL.Query().Get("q"), cursor)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleRecordCatalogDisplays(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.RecordDisplays(r.Context(), input.GalleryIDs); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCatalogCatchUp(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryID int64 `json:"gallery_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.CatchUp(r.Context(), input.GalleryID); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleSetCatalogVerdict(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		galleryID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			catalogError(w, r, catalog.ErrInvalidGalleryReference)
			return
		}
		var input struct {
			Verdict string `json:"verdict"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.SetVerdict(r.Context(), galleryID, input.Verdict); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleRemoveCatalogVerdict(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		galleryID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			catalogError(w, r, catalog.ErrInvalidGalleryReference)
			return
		}
		if err := service.RemoveVerdict(r.Context(), galleryID); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func catalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalidFactsBatch), errors.Is(err, catalog.ErrInvalidDisplays):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_batch"})
	case errors.Is(err, panda.ErrInvalidCategory):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_category"})
	case errors.Is(err, catalog.ErrInvalidGalleryReference):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gallery_reference"})
	case errors.Is(err, catalog.ErrInvalidVerdict):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_verdict"})
	case errors.Is(err, catalog.ErrGalleryNotInCatalog):
		server.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "gallery_not_found"})
	case errors.Is(err, catalog.ErrInvalidQuery):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	case errors.Is(err, catalog.ErrInvalidPagination):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
