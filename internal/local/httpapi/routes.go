package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(logger *slog.Logger, libraries *library.Service) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/libraries", HandleCreateLibrary(libraries, logger))
	mux.Handle("GET /api/libraries", HandleListLibraries(libraries, logger))
	mux.Handle("GET /api/libraries/{id}", HandleGetLibrary(libraries, logger))
	mux.Handle("PATCH /api/libraries/{id}", HandleRenameLibrary(libraries, logger))
	mux.Handle("DELETE /api/libraries/{id}", HandleDeleteLibrary(libraries, logger))
	mux.Handle("POST /api/libraries/{id}/availability-check", HandleCheckLibraryAvailability(libraries, logger))
	mux.HandleFunc("/api/libraries", methodNotAllowed("GET, HEAD, POST"))
	mux.HandleFunc("/api/libraries/{id}", methodNotAllowed("GET, HEAD, PATCH, DELETE"))
	mux.HandleFunc("/api/libraries/{id}/availability-check", methodNotAllowed("POST"))
	mux.HandleFunc("GET /healthz", server.Health)
	mux.HandleFunc("/healthz", server.HealthMethodNotAllowed)
	mux.HandleFunc("/", server.NotFound)
	return server.Logging(logger, mux)
}
