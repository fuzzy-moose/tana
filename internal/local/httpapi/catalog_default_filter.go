package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/catalogfilter"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandlePandaDefaultFilter(store *catalogfilter.Store, client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if store == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "settings_unavailable"})
			return
		}
		if r.Method == http.MethodPut {
			var input catalogfilter.Filter
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			filter, err := catalogfilter.Normalize(input)
			if err != nil {
				server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
				return
			}
			// Validate against the collector's namespace vocabulary before saving a
			// query that will constrain every subsequent catalog visit.
			if filter.Query != "" {
				if !catalogCollectorConfigured(w, client) {
					return
				}
				if _, err := client.Catalog(r.Context(), collectorapi.CatalogOptions{Query: filter.Query, PageSize: 1}); err != nil {
					writeCatalogCollectorError(w, err)
					return
				}
			}
			if err := store.Save(r.Context(), filter); err != nil {
				writeDefaultFilterError(w, r, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, filter)
			return
		}
		filter, err := store.Load(r.Context())
		if err != nil {
			writeDefaultFilterError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, filter)
	})
}

func writeDefaultFilterError(w http.ResponseWriter, r *http.Request, err error) {
	server.GetHTTPContext(r).Err = err
	server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
}
