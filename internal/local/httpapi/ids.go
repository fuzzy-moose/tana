package httpapi

import (
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/server"
)

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		server.NotFound(w, r)
		return 0, false
	}
	return id, true
}
