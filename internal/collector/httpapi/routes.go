package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/fuzzy-moose/tana/internal/collector"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(app *collector.App) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/status", HandleStatus(app.Status))
	mux.Handle("POST /api/favorites/{category}/sync", HandleSyncFavorites(app.Favorites))
	mux.Handle("POST /api/metadata/lookup", HandleLookupMetadata(app.Metadata))
	mux.Handle("POST /api/metadata/fetches", HandleRequestFetch(app.Metadata))
	mux.Handle("GET /api/metadata/fetches/{id}", HandleGetFetchJob(app.Metadata))
	mux.HandleFunc("GET /healthz", server.Health)
	var handler http.Handler = mux
	handler = server.CSRFMiddleware(handler)
	handler = authenticate(app.APIToken, handler)
	handler = server.LoggingMiddleware(app.Logger, handler)
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
