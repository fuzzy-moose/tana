package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/sitemap"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleSitemap(service *sitemap.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result collectorapi.SitemapStatus
		var err error
		status := http.StatusAccepted
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			result, err = service.Status(r.Context())
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
				result, err = service.Start(r.Context(), input.Force)
			case "cancel", "retry":
				var input struct{}
				if !server.DecodeJSON(w, r, &input) {
					return
				}
				if r.PathValue("action") == "cancel" {
					result, err = service.Cancel(r.Context())
				} else {
					result, err = service.Retry(r.Context())
				}
			default:
				http.NotFound(w, r)
				return
			}
		}
		if err != nil {
			code := "sitemap_unavailable"
			status = http.StatusInternalServerError
			if errors.Is(err, sitemap.ErrState) {
				status, code = http.StatusConflict, "sitemap_state_conflict"
			} else {
				server.GetHTTPContext(r).Err = err
			}
			server.WriteJSON(w, status, map[string]string{"error": code})
			return
		}
		server.WriteJSON(w, status, result)
	})
}
