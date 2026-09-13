package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collector/metadata"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleLookupGallery(service *catalog.Service, fetches *metadata.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ref panda.GalleryRef
		if !server.DecodeJSON(w, r, &ref) {
			return
		}
		if ref.ID <= 0 || strings.IndexFunc(ref.Token, unicode.IsSpace) >= 0 {
			catalogError(w, r, catalog.ErrInvalidGalleryReference)
			return
		}
		result, err := service.Lookup(r.Context(), ref.ID)
		if errors.Is(err, sql.ErrNoRows) {
			if ref.Token == "" {
				server.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "gallery_not_found"})
				return
			}
			job, err := fetches.RequestFetch(r.Context(), []panda.GalleryRef{ref})
			if err != nil {
				metadataError(w, r, err)
				return
			}
			result = collectorapi.GalleryLookupResult{GalleryID: ref.ID, Token: ref.Token,
				URL: service.GalleryURL(ref), Unverified: true, FetchJob: &job}
			server.WriteJSON(w, http.StatusAccepted, result)
			return
		}
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}
