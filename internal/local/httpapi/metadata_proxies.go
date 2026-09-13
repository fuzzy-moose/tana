package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorMetadataProxies(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var result collectorapi.MetadataProxyStatus
		var err error
		switch {
		case r.Method == http.MethodGet:
			result, err = client.MetadataProxies(r.Context())
		case r.Method == http.MethodPut && r.PathValue("id") == "":
			var settings collectorapi.MetadataProxySettings
			if !server.DecodeJSON(w, r, &settings) {
				return
			}
			result, err = client.SetMetadataProxies(r.Context(), settings)
		case r.Method == http.MethodDelete:
			result, err = client.DeleteMetadataProxy(r.Context(), r.PathValue("id"))
		default:
			var input collectorapi.MetadataProxyInput
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			result, err = client.SaveMetadataProxy(r.Context(), r.PathValue("id"), input)
		}
		if err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			switch {
			case errors.Is(err, collectorapi.ErrInvalidProxy):
				status, code = http.StatusBadRequest, err.Error()
			case errors.Is(err, collectorapi.ErrDuplicateProxy), errors.Is(err, collectorapi.ErrProxyChanging):
				status, code = http.StatusConflict, err.Error()
			case errors.Is(err, collectorapi.ErrProxyNotFound):
				status, code = http.StatusNotFound, err.Error()
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
