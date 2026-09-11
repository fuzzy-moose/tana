package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorSitemap(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if client == nil {
			server.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_not_configured"})
			return
		}
		var result collectorapi.SitemapStatus
		var err error
		status := http.StatusAccepted
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			result, err = client.SitemapStatus(r.Context())
			status = http.StatusOK
		} else {
			switch r.PathValue("action") {
			case "start":
				var input struct {
					Force bool `json:"force"`
				}
				if !server.DecodeJSON(w, r, &input) {
					return
				}
				result, err = client.StartSitemap(r.Context(), input.Force)
			case "cancel", "retry":
				var input struct{}
				if !server.DecodeJSON(w, r, &input) {
					return
				}
				if r.PathValue("action") == "cancel" {
					result, err = client.CancelSitemap(r.Context())
				} else {
					result, err = client.RetrySitemap(r.Context())
				}
			default:
				http.NotFound(w, r)
				return
			}
		}
		if err != nil {
			status, code := http.StatusBadGateway, "collector_unavailable"
			if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
				switch upstream.StatusCode {
				case http.StatusUnauthorized, http.StatusForbidden:
					code = "collector_unauthorized"
				case http.StatusConflict:
					status, code = http.StatusConflict, "sitemap_state_conflict"
				}
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, status, result)
	})
}
