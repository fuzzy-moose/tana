package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(logger *slog.Logger, libraries *library.Service) http.Handler {
	mux := http.NewServeMux()
	api := libraryAPI{libraries: libraries, logger: logger}
	mux.HandleFunc("POST /api/libraries", api.create)
	mux.HandleFunc("GET /api/libraries", api.list)
	mux.HandleFunc("GET /api/libraries/{id}", api.get)
	mux.HandleFunc("PATCH /api/libraries/{id}", api.rename)
	mux.HandleFunc("DELETE /api/libraries/{id}", api.delete)
	mux.HandleFunc("POST /api/libraries/{id}/availability-check", api.check)
	mux.HandleFunc("/api/libraries", methodNotAllowed("GET, HEAD, POST"))
	mux.HandleFunc("/api/libraries/{id}", methodNotAllowed("GET, HEAD, PATCH, DELETE"))
	mux.HandleFunc("/api/libraries/{id}/availability-check", methodNotAllowed("POST"))
	mux.HandleFunc("GET /healthz", server.Health)
	mux.HandleFunc("/healthz", server.HealthMethodNotAllowed)
	mux.HandleFunc("/", server.NotFound)
	return server.Logging(logger, mux)
}
