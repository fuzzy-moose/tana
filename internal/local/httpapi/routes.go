package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/scan"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(logger *slog.Logger, libraries *library.Service, scans *scan.Service, galleries *gallery.SQLiteRepository) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/galleries", HandleListGalleries(galleries))
	mux.Handle("GET /api/galleries/{id}", HandleGetGallery(galleries))
	mux.Handle("GET /api/galleries/{id}/pages/{number}/image", HandleGalleryImage(galleries))
	mux.HandleFunc("/api/galleries", methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("/api/galleries/{id}", methodNotAllowed("GET, HEAD"))
	mux.HandleFunc("/api/galleries/{id}/pages/{number}/image", methodNotAllowed("GET, HEAD"))
	mux.Handle("POST /api/scans", HandleRequestScan(scans))
	mux.Handle("GET /api/scans/status", HandleScanStatus(scans))
	mux.HandleFunc("/api/scans", methodNotAllowed("POST"))
	mux.HandleFunc("/api/scans/status", methodNotAllowed("GET, HEAD"))
	mux.Handle("POST /api/libraries", HandleCreateLibrary(libraries))
	mux.Handle("GET /api/libraries", HandleListLibraries(libraries))
	mux.Handle("GET /api/libraries/{id}", HandleGetLibrary(libraries))
	mux.Handle("PATCH /api/libraries/{id}", HandleRenameLibrary(libraries))
	mux.Handle("DELETE /api/libraries/{id}", HandleDeleteLibrary(libraries))
	mux.Handle("POST /api/libraries/{id}/availability-check", HandleCheckLibraryAvailability(libraries))
	mux.HandleFunc("/api/libraries", methodNotAllowed("GET, HEAD, POST"))
	mux.HandleFunc("/api/libraries/{id}", methodNotAllowed("GET, HEAD, PATCH, DELETE"))
	mux.HandleFunc("/api/libraries/{id}/availability-check", methodNotAllowed("POST"))
	mux.HandleFunc("GET /healthz", server.Health)
	mux.HandleFunc("/healthz", server.HealthMethodNotAllowed)
	mux.HandleFunc("/", server.NotFound)
	var handler http.Handler = mux
	handler = server.CSRFMiddleware(handler)
	handler = server.LoggingMiddleware(logger, handler)
	handler = server.HTTPContextMiddleware(handler)
	return handler
}
