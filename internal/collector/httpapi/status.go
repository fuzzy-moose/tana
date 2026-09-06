package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector/status"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleStatus(service *status.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, err := service.Get(r.Context())
		if err != nil {
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "status_unavailable"})
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
