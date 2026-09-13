package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleImportCollectorMetadataProxies(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var input collectorapi.MetadataProxyImportInput
		if !server.DecodeJSONLimit(w, r, &input, 2<<20) {
			return
		}
		result, err := client.ImportMetadataProxies(r.Context(), input)
		if err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			switch {
			case errors.Is(err, collectorapi.ErrInvalidProxyList):
				status, code = http.StatusBadRequest, err.Error()
			case errors.Is(err, collectorapi.ErrProxyListTooLarge):
				status, code = http.StatusRequestEntityTooLarge, err.Error()
			default:
				if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok &&
					(upstream.StatusCode == http.StatusUnauthorized || upstream.StatusCode == http.StatusForbidden) {
					code = "collector_unauthorized"
				}
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
