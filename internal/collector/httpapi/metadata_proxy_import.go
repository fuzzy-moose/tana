package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/metadataproxy"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleImportMetadataProxies(service *metadataproxy.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "proxy_channels_unavailable"})
			return
		}
		var input collectorapi.MetadataProxyImportInput
		if !server.DecodeJSONLimit(w, r, &input, 2<<20) {
			return
		}
		result, err := service.Import(r.Context(), input)
		if err != nil {
			status, code := http.StatusServiceUnavailable, "proxy_channels_unavailable"
			switch {
			case errors.Is(err, collectorapi.ErrInvalidProxyList):
				status, code = http.StatusBadRequest, err.Error()
			case errors.Is(err, collectorapi.ErrProxyListTooLarge):
				status, code = http.StatusRequestEntityTooLarge, err.Error()
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
