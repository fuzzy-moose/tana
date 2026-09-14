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
	mux.Handle("GET /api/status", HandleStatus())
	mux.Handle("GET /api/inventory/status", HandleInventoryStatus(app.Status))
	mux.Handle("GET /api/favorites/status", HandleFavoritesStatus(app.Status))
	mux.Handle("GET /api/metadata/status", HandleMetadataStatus(app.Status))
	mux.Handle("POST /api/metadata/pause", HandleMetadataCollectionPause(app.Metadata, true))
	mux.Handle("POST /api/metadata/resume", HandleMetadataCollectionPause(app.Metadata, false))
	proxies := HandleMetadataProxies(app.MetadataProxies)
	mux.Handle("GET /api/metadata/proxies", proxies)
	mux.Handle("PUT /api/metadata/proxies", proxies)
	mux.Handle("POST /api/metadata/proxies/import", HandleImportMetadataProxies(app.MetadataProxies))
	mux.Handle("POST /api/metadata/proxies/channels", proxies)
	mux.Handle("PUT /api/metadata/proxies/channels/{id}", proxies)
	mux.Handle("DELETE /api/metadata/proxies/channels/{id}", proxies)
	mux.Handle("GET /api/catalog", HandleCatalog(app.Catalog))
	mux.Handle("POST /api/catalog/lookup", HandleLookupGallery(app.Catalog, app.Metadata))
	mux.Handle("GET /api/catalog/completions", HandleCompleteCatalog(app.Catalog))
	mux.Handle("GET /api/feed/status", HandleFeed(app.Feeds))
	mux.Handle("POST /api/feed/refresh", HandleFeed(app.Feeds))
	mux.Handle("GET /api/feed/captures", HandleListFeedCaptures(app.Feeds))
	mux.Handle("GET /api/feed/captures/{id}/file", HandleFeedCaptureFile(app.Feeds))
	mux.Handle("GET /api/sitemap/status", HandleSitemap(app.Sitemap, app.Ban))
	mux.Handle("POST /api/sitemap/{action}", HandleSitemap(app.Sitemap, app.Ban))
	mux.Handle("POST /api/favorites/{category}/sync", HandleSyncFavorites(app.Favorites))
	mux.Handle("GET /api/favorites/{category}/download-candidates", HandleFavoriteDownloadCandidates(app.Favorites))
	mux.Handle("GET /api/favorites/download-settings", HandleFavoriteDownloadSettings(app.Favorites))
	mux.Handle("PUT /api/favorites/download-settings", HandleFavoriteDownloadSettings(app.Favorites))
	mux.Handle("POST /api/metadata/lookup", HandleLookupMetadata(app.Metadata))
	mux.Handle("POST /api/metadata/fetches", HandleRequestFetch(app.Metadata))
	mux.Handle("GET /api/metadata/fetches/{id}", HandleGetFetchJob(app.Metadata))
	mux.Handle("POST /api/reference-imports", HandleAcceptReferenceImport(app.ReferenceImports))
	mux.Handle("GET /api/reference-imports", HandleListReferenceImports(app.ReferenceImports))
	mux.Handle("GET /api/reference-imports/{id}", HandleGetReferenceImport(app.ReferenceImports))
	mux.Handle("POST /api/reference-imports/{id}/pause", HandlePauseReferenceImport(app.ReferenceImports))
	mux.Handle("POST /api/reference-imports/{id}/resume", HandleResumeReferenceImport(app.ReferenceImports))
	mux.Handle("POST /api/reference-imports/{id}/cancel", HandleCancelReferenceImport(app.ReferenceImports))
	mux.Handle("POST /api/reference-imports/{id}/retry", HandleRetryReferenceImport(app.ReferenceImports))
	mux.Handle("POST /api/downloads", HandleSubmitDownload(app.Downloads))
	mux.Handle("POST /api/downloads/batch", HandleSubmitDownloadBatch(app.Downloads))
	mux.Handle("GET /api/downloads", HandleListDownloads(app.Downloads))
	mux.Handle("GET /api/downloads/completed", HandleCompletedDownloads(app.Downloads))
	mux.Handle("GET /api/downloads/{id}", HandleGetDownload(app.Downloads))
	mux.Handle("POST /api/downloads/{id}/retry", HandleRetryDownload(app.Downloads))
	mux.Handle("POST /api/downloads/{id}/cancel", HandleCancelDownload(app.Downloads))
	mux.Handle("DELETE /api/downloads/{id}", HandleDeleteDownload(app.Downloads))
	mux.Handle("GET /api/downloads/{id}/file", HandleGetDownloadFile(app.Downloads))
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
