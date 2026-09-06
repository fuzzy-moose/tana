package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(logger *slog.Logger, service *metadata.Service, token string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/metadata/lookup", lookup(service))
	mux.HandleFunc("/api/metadata/lookup", methodNotAllowed("POST"))
	mux.HandleFunc("POST /api/metadata/fetches", requestFetch(service))
	mux.HandleFunc("/api/metadata/fetches", methodNotAllowed("POST"))
	mux.HandleFunc("GET /api/metadata/fetches/{id}", getFetchJob(service))
	mux.HandleFunc("/api/metadata/fetches/{id}", methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("GET /healthz", server.Health)
	mux.HandleFunc("/healthz", server.HealthMethodNotAllowed)
	mux.HandleFunc("/", server.NotFound)
	var handler http.Handler = mux
	handler = server.CSRFMiddleware(handler)
	handler = authenticate(token, handler)
	handler = server.LoggingMiddleware(logger, handler)
	handler = server.HTTPContextMiddleware(handler)
	return handler
}

func authenticate(token string, next http.Handler) http.Handler {
	expected := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			parts := strings.Split(r.Header.Get("Authorization"), " ")
			valid := false
			if token != "" && len(r.Header.Values("Authorization")) == 1 && len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				provided := sha256.Sum256([]byte(parts[1]))
				valid = subtle.ConstantTimeCompare(expected[:], provided[:]) == 1
			}
			if !valid {
				w.Header().Set("WWW-Authenticate", `Bearer realm="collector"`)
				server.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func methodNotAllowed(allow string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", allow)
		server.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
	}
}
