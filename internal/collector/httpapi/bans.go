package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleResetBan(bans *collector.Bans) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct{}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := bans.Reset(r.Context(), r.PathValue("group")); err != nil {
			if errors.Is(err, collector.ErrUnknownBanGroup) {
				server.NotFound(w, r)
				return
			}
			server.GetHTTPContext(r).Err = err
			server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
