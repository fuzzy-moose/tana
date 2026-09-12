package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/feed"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleFeed(service *feed.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result collectorapi.FeedStatus
		var err error
		status := http.StatusOK
		if r.Method == http.MethodPost {
			var input struct{}
			if !server.DecodeJSON(w, r, &input) {
				return
			}
			result, err = service.Refresh(r.Context())
			status = http.StatusAccepted
		} else {
			result, err = service.Status(r.Context())
		}
		if err != nil {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "feed_unavailable"})
			return
		}
		server.WriteJSON(w, status, result)
	})
}
