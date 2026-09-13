package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/metadataproxy"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleMetadataProxies(service *metadataproxy.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "proxy_channels_unavailable"})
			return
		}
		var err error
		switch {
		case r.Method == http.MethodPut && r.PathValue("id") == "":
			var settings collectorapi.MetadataProxySettings
			if !server.DecodeJSON(w, r, &settings) {
				return
			}
			err = service.SetSettings(r.Context(), settings)
		case r.Method == http.MethodPost || r.Method == http.MethodPut:
			var input collectorapi.MetadataProxyInput
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			err = service.Save(r.Context(), r.PathValue("id"), input)
		case r.Method == http.MethodDelete:
			err = service.Delete(r.Context(), r.PathValue("id"))
		}
		if err != nil {
			status, code := http.StatusServiceUnavailable, "proxy_channels_unavailable"
			switch {
			case errors.Is(err, collectorapi.ErrInvalidProxy):
				status, code = http.StatusBadRequest, err.Error()
			case errors.Is(err, collectorapi.ErrDuplicateProxy), errors.Is(err, collectorapi.ErrProxyChanging):
				status, code = http.StatusConflict, err.Error()
			case errors.Is(err, collectorapi.ErrProxyNotFound):
				status, code = http.StatusNotFound, err.Error()
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		status, err := service.Status(r.Context())
		if err != nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "proxy_channels_unavailable"})
			return
		}
		server.WriteJSON(w, http.StatusOK, status)
	})
}
