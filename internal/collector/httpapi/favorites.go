package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collector/favorites"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleSyncFavorites(service *favorites.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		category, err := strconv.Atoi(r.PathValue("category"))
		if err != nil || category < 0 || category > 9 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_category"})
			return
		}
		var input struct {
			Full bool `json:"full"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.Enqueue(category, input.Full); err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, favorites.ErrInvalidCategory) {
				status = http.StatusBadRequest
			}
			server.WriteJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}
