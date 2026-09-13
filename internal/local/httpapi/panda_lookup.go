package httpapi

import (
	"errors"
	"net/http"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorGalleryLookup(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var ref panda.GalleryRef
		if !server.DecodeJSON(w, r, &ref) {
			return
		}
		result, err := client.LookupGallery(r.Context(), ref)
		if err != nil {
			writeGalleryLookupError(w, err, false)
			return
		}
		status := http.StatusOK
		if result.FetchJob != nil {
			status = http.StatusAccepted
		}
		server.WriteJSON(w, status, result)
	})
}

func HandleCollectorMetadataFetch(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		job, err := client.GetMetadataFetch(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGalleryLookupError(w, err, true)
			return
		}
		server.WriteJSON(w, http.StatusOK, job)
	})
}

func writeGalleryLookupError(w http.ResponseWriter, err error, fetch bool) {
	status, code := http.StatusBadGateway, "collector_unavailable"
	if upstream, ok := errors.AsType[*collectorapi.HTTPError](err); ok {
		switch upstream.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "collector_unauthorized"
		case http.StatusBadRequest:
			if upstream.Code == "invalid_gallery_reference" {
				status, code = http.StatusBadRequest, upstream.Code
			}
		case http.StatusNotFound:
			if fetch {
				status, code = http.StatusNotFound, "metadata_fetch_not_found"
			} else if upstream.Code == "gallery_not_found" {
				status, code = http.StatusNotFound, upstream.Code
			}
		}
	}
	server.WriteJSON(w, status, map[string]string{"error": code})
}
