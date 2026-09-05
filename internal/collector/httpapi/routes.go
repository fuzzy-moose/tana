package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.Health)
	mux.HandleFunc("/healthz", server.HealthMethodNotAllowed)
	mux.HandleFunc("/", server.NotFound)
	var handler http.Handler = mux
	handler = server.CSRFMiddleware(handler)
	handler = server.LoggingMiddleware(logger, handler)
	handler = server.HTTPContextMiddleware(handler)
	return handler
}
