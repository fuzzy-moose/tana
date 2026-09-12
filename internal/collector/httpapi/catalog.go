package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCatalog(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		options := collectorapi.CatalogOptions{Query: r.URL.Query().Get("q"), Page: 1, PageSize: 24}
		for _, parameter := range []struct {
			name string
			dest *int
		}{{"page", &options.Page}, {"page_size", &options.PageSize}} {
			if r.URL.Query().Has(parameter.name) {
				value, err := strconv.Atoi(r.URL.Query().Get(parameter.name))
				if err != nil {
					catalogError(w, r, catalog.ErrInvalidPagination)
					return
				}
				*parameter.dest = value
			}
		}
		if r.URL.Query().Has("include_expunged") {
			var err error
			options.IncludeExpunged, err = strconv.ParseBool(r.URL.Query().Get("include_expunged"))
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

func catalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalidQuery):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	case errors.Is(err, catalog.ErrInvalidPagination):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
