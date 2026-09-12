package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorCatalog(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		page, ok := positiveQuery(r, "page", 1)
		pageSize, sizeOK := positiveQuery(r, "page_size", 24)
		if !ok || !sizeOK || pageSize > 100 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			return
		}
		includeExpunged := false
		if r.URL.Query().Has("include_expunged") {
			var err error
			includeExpunged, err = strconv.ParseBool(r.URL.Query().Get("include_expunged"))
			if err != nil {
				server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
				return
			}
		}
		result, err := client.Catalog(r.Context(), collectorapi.CatalogOptions{
			Query: r.URL.Query().Get("q"), Page: int(page), PageSize: int(pageSize), IncludeExpunged: includeExpunged,
		})
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCollectorCatalogCompletions(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		cursor, err := strconv.Atoi(r.URL.Query().Get("cursor"))
		if err != nil || cursor < 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
			return
		}
		result, err := client.CompleteCatalog(r.Context(), r.URL.Query().Get("q"), cursor)
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCollectorFeed(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var result collectorapi.FeedStatus
		var err error
		status := http.StatusOK
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			result, err = client.FeedStatus(r.Context())
		} else {
			var input struct{}
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			result, err = client.RefreshFeed(r.Context())
			status = http.StatusAccepted
		}
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, status, result)
	})
}

func catalogCollectorConfigured(w http.ResponseWriter, client *collectorapi.Client) bool {
	if client != nil {
		return true
	}
	server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
	return false
}

func writeCatalogCollectorError(w http.ResponseWriter, err error) {
	status, code := http.StatusBadGateway, "collector_unavailable"
	if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
		switch upstream.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "collector_unauthorized"
		case http.StatusBadRequest:
			if upstream.Code == "invalid_query" || upstream.Code == "invalid_pagination" {
				status, code = http.StatusBadRequest, upstream.Code
			}
		}
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
