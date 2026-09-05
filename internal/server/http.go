package server

import (
	"net/http"
)

// Health reports process liveness, independently of upstream metadata availability.
func Health(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
}

// HealthMethodNotAllowed preserves method handling alongside the JSON fallback.
func HealthMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", "GET, HEAD")
	WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
}
