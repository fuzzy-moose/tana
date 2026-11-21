package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleListGalleries(galleries *gallery.Store, client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, ok := positiveQuery(r, "page", 1)
		pageSize, sizeOK := positiveQuery(r, "page_size", 24)
		if !ok || !sizeOK || pageSize > 100 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
			return
		}
		categories, err := panda.NormalizeCategories(r.URL.Query()["category"])
		search := strings.TrimSpace(r.URL.Query().Get("q"))
		options := gallery.BrowseOptions{Categories: categories, Sort: gallery.BrowseSort(r.URL.Query().Get("sort"))}
		if err != nil || !options.Sort.Valid() {
			writeGalleryError(w, r, gallery.ErrInvalidQuery)
			return
		}
		// The Panda gallery category of each candidate the collector answered for.
		var pandaCategories map[int64]string
		if options.NeedsPanda() {
			if !catalogCollectorConfigured(w, client) {
				return
			}
			candidates, err := galleries.PandaCandidates(r.Context(), search)
			if err != nil {
				writeGalleryError(w, r, err)
				return
			}
			ids := make([]int64, 0, len(candidates))
			for _, candidate := range candidates {
				ids = append(ids, candidate.PandaID)
			}
			facts, err := client.CatalogFacts(r.Context(), ids)
			if err != nil {
				writeCatalogCollectorError(w, err)
				return
			}
			byID := make(map[int64]collectorapi.CatalogFact, len(facts))
			for _, fact := range facts {
				byID[fact.GalleryID] = fact
			}
			pandaCategories = categoriesOf(facts)
			for _, candidate := range candidates {
				fact := byID[candidate.PandaID]
				options.PandaFacts = append(options.PandaFacts, gallery.PandaFact{
					SourceID: candidate.SourceID, Category: fact.Category, FavoritedAt: fact.FavoritedAt,
				})
			}
		}
		result, err := galleries.BrowseFiltered(r.Context(), search, page, pageSize, options)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		if client != nil {
			if err := addPandaCategories(r, galleries, client, result.Items, pandaCategories); err != nil {
				writeGalleryError(w, r, err)
				return
			}
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

// pandaCategoryTimeout bounds the collector call of a plain listing: a category label must not hold up browsing.
const pandaCategoryTimeout = 2 * time.Second

// categoriesOf maps each Panda gallery ID to its Panda gallery category.
func categoriesOf(facts []collectorapi.CatalogFact) map[int64]string {
	categories := make(map[int64]string, len(facts))
	for _, fact := range facts {
		categories[fact.GalleryID] = fact.Category
	}
	return categories
}

// addPandaCategories labels the listed galleries with the categories of their Panda gallery candidates.
// Known categories come from facts the request already fetched; otherwise the collector is asked for this
// page only, and a collector failure or delay leaves the categories unknown instead of failing the listing.
func addPandaCategories(r *http.Request, galleries *gallery.Store, client *collectorapi.Client, items []gallery.ListedGallery, known map[int64]string) error {
	galleryIDs := make([]int64, 0, len(items))
	for _, item := range items {
		galleryIDs = append(galleryIDs, item.ID)
	}
	candidates, err := galleries.PandaCandidateIDsByGallery(r.Context(), galleryIDs)
	if err != nil || len(candidates) == 0 {
		return err
	}
	if known == nil {
		ids := make([]int64, 0, len(candidates))
		for _, id := range galleryIDs {
			if candidate, ok := candidates[id]; ok {
				ids = append(ids, candidate)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), pandaCategoryTimeout)
		defer cancel()
		facts, err := client.CatalogFacts(ctx, ids)
		if err != nil {
			return nil
		}
		known = categoriesOf(facts)
	}
	for i := range items {
		if id, ok := candidates[items[i].ID]; ok {
			items[i].PandaCategory = known[id]
		}
	}
	return nil
}

func positiveQuery(r *http.Request, key string, fallback int64) (int64, bool) {
	if !r.URL.Query().Has(key) {
		return fallback, true
	}
	n, err := strconv.ParseInt(r.URL.Query().Get(key), 10, 64)
	return n, err == nil && n > 0
}

func HandleCompleteGallerySearch(galleries *gallery.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor, err := strconv.Atoi(r.URL.Query().Get("cursor"))
		if err != nil {
			writeGalleryError(w, r, gallery.ErrInvalidQuery)
			return
		}
		result, err := galleries.Complete(r.Context(), r.URL.Query().Get("q"), cursor)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGetGallery(galleries *gallery.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		result, err := galleries.Detail(r.Context(), id)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleGalleryImage(galleries *gallery.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}

		number, err := strconv.ParseInt(r.PathValue("number"), 10, 64)
		if err != nil || number < 1 {
			server.NotFound(w, r)
			return
		}
		content, err := galleries.OpenImage(r.Context(), id, number)
		if err != nil {
			writeGalleryError(w, r, err)
			return
		}
		defer content.Close()
		// Removing source files can renumber gallery occurrences at this URL.
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", content.ContentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Length", strconv.FormatInt(content.Size, 10))
		if r.Method != http.MethodHead {
			if _, err := io.Copy(w, content); err != nil {
				server.GetHTTPContext(r).Err = err
			}
		}
	})
}

func writeGalleryError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, gallery.ErrInvalidQuery):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	case errors.Is(err, gallery.ErrNotFound):
		server.NotFound(w, r)
	case errors.Is(err, gallery.ErrImageUnavailable):
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "image_unavailable"})
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
