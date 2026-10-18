package server

import (
	"net/http"
)

// Health reports process liveness, independently of upstream metadata availability.
func Health(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func NotFound(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
}
