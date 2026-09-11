package httpapi

import (
	"net/http"

	"github.com/fuzzy-moose/tana/internal/local"
	"github.com/fuzzy-moose/tana/internal/server"
)

func NewHandler(app *local.App) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/collector/status", HandleCollectorStatus(app.Collector))
	mux.Handle("POST /api/collector/favorites/{category}/sync", HandleCollectorSyncFavorites(app.Collector))
	mux.Handle("GET /api/collector/favorites/download-settings", HandleCollectorFavoriteDownloadSettings(app.Collector))
	mux.Handle("PUT /api/collector/favorites/download-settings", HandleCollectorFavoriteDownloadSettings(app.Collector))
	downloads := HandleCollectorDownloads(app.Collector)
	mux.Handle("/api/collector/downloads", downloads)
	mux.Handle("/api/collector/downloads/", downloads)
	mux.Handle("GET /api/galleries", HandleListGalleries(app.Galleries))
	mux.Handle("GET /api/gallery-search/completions", HandleCompleteGallerySearch(app.Galleries))
	mux.Handle("GET /api/galleries/{id}", HandleGetGallery(app.Galleries))
	mux.Handle("GET /api/galleries/{id}/pages/{number}/image", HandleGalleryImage(app.Galleries))
	mux.Handle("POST /api/scans", HandleRequestScan(app.Scans))
	mux.Handle("GET /api/scans/status", HandleScanStatus(app.Scans))
	mux.Handle("POST /api/libraries", HandleCreateLibrary(app.Libraries))
	mux.Handle("GET /api/libraries", HandleListLibraries(app.Libraries))
	mux.Handle("GET /api/libraries/{id}", HandleGetLibrary(app.Libraries))
	mux.Handle("PATCH /api/libraries/{id}", HandleRenameLibrary(app.Libraries))
	mux.Handle("DELETE /api/libraries/{id}", HandleDeleteLibrary(app.Libraries))
	mux.Handle("POST /api/libraries/{id}/availability-check", HandleCheckLibraryAvailability(app.Libraries))
	mux.HandleFunc("GET /healthz", server.Health)

	root := http.NewServeMux()
	root.Handle("/api", mux)
	root.Handle("/api/", mux)
	root.Handle("/healthz", mux)
	if app.Web != nil {
		files := http.NewServeMux()
		files.Handle("GET /", http.FileServerFS(app.Web))
		root.Handle("/", files)
	}
	var handler http.Handler = root
	handler = server.CSRFMiddleware(handler)
	handler = server.LoggingMiddleware(app.Logger, handler)
	handler = server.HTTPContextMiddleware(handler)
	return handler
}
