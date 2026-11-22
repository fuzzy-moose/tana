package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCatalog(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		options := collectorapi.CatalogOptions{
			Query: r.URL.Query().Get("q"), Categories: r.URL.Query()["category"],
			DefaultQuery: r.URL.Query().Get("default_q"), DefaultCategories: r.URL.Query()["default_category"],
			Cursor: r.URL.Query().Get("cursor"), PageSize: 24,
			ScoreFilter: r.URL.Query().Get("score_filter"),
		}
		if r.URL.Query().Has("page_size") {
			value, err := strconv.Atoi(r.URL.Query().Get("page_size"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidPagination)
				return
			}
			options.PageSize = value
		}
		if r.URL.Query().Has("include_expunged") {
			var err error
			options.IncludeExpunged, err = strconv.ParseBool(r.URL.Query().Get("include_expunged"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidQuery)
				return
			}
		}
		if r.URL.Query().Has("unseen") {
			var err error
			options.Unseen, err = strconv.ParseBool(r.URL.Query().Get("unseen"))
			if err != nil {
				catalogError(w, r, catalog.ErrInvalidQuery)
				return
			}
		}
		result, err := service.List(r.Context(), options)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleCompleteCatalog(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor, err := strconv.Atoi(r.URL.Query().Get("cursor"))
		if err != nil {
			catalogError(w, r, catalog.ErrInvalidQuery)
			return
		}
		result, err := service.Complete(r.Context(), r.URL.Query().Get("q"), cursor)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, result)
	})
}

func HandleRecordCatalogDisplays(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryIDs []int64 `json:"gallery_ids"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.RecordDisplays(r.Context(), input.GalleryIDs); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCatalogCatchUp(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			GalleryID int64 `json:"gallery_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.CatchUp(r.Context(), input.GalleryID); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// HandleSetScorePin answers with the stored pin. A body without a weight, or
// with a null weight, is an invalid pin.
func HandleSetScorePin(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Kind   string   `json:"kind"`
			Value  string   `json:"value"`
			Weight *float64 `json:"weight"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if input.Weight == nil {
			catalogError(w, r, catalog.ErrInvalidScorePin)
			return
		}
		pin, err := service.SetScorePin(r.Context(), input.Kind, input.Value, *input.Weight)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, pin)
	})
}

func HandleRemoveScorePin(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			catalogError(w, r, catalog.ErrInvalidScorePin)
			return
		}
		if err := service.RemoveScorePin(r.Context(), id); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandlePreviewScoreModel(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var options collectorapi.ScorePreviewOptions
		if !server.DecodeJSON(w, r, &options) {
			return
		}
		preview, err := service.PreviewScoreModel(r.Context(), options)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, preview)
	})
}

func HandleApplyScoreModel(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			CandidateID     int64 `json:"candidate_id"`
			ThresholdTenths int   `json:"threshold_tenths"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.ApplyScoreModel(r.Context(), input.CandidateID, input.ThresholdTenths); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleSetScoreThreshold(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			ThresholdTenths int `json:"threshold_tenths"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.SetScoreThreshold(r.Context(), input.ThresholdTenths); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// HandleSetAuthorBonus sets the author bonus; a null or absent bonus turns it
// off.
func HandleSetAuthorBonus(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Bonus *float64 `json:"bonus"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := service.SetAuthorBonus(r.Context(), input.Bonus); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// HandleScoreTagWeights answers with the effective weight and the pin of each
// sent tag, in the sent order.
func HandleScoreTagWeights(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Tags []string `json:"tags"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		weights, err := service.ScoreTagWeights(r.Context(), input.Tags)
		if err != nil {
			catalogError(w, r, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, weights)
	})
}

func HandleRemoveScoreModel(service *catalog.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := service.RemoveScoreModel(r.Context()); err != nil {
			catalogError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func catalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrInvalidFactsBatch), errors.Is(err, catalog.ErrInvalidDisplays):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_batch"})
	case errors.Is(err, panda.ErrInvalidCategory):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_category"})
	case errors.Is(err, catalog.ErrInvalidGalleryReference):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_gallery_reference"})
	case errors.Is(err, catalog.ErrInvalidScorePin):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pin"})
	case errors.Is(err, catalog.ErrInvalidAuthorBonus):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_author_bonus"})
	case errors.Is(err, catalog.ErrTagNotFound):
		server.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "tag_not_found"})
	case errors.Is(err, catalog.ErrInvalidScoreThreshold):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_threshold"})
	case errors.Is(err, catalog.ErrScoreCandidateNotFound):
		server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "score_candidate_not_found"})
	case errors.Is(err, catalog.ErrNoActiveScoreModel):
		server.WriteJSON(w, http.StatusConflict, map[string]string{"error": "score_model_not_active"})
	case errors.Is(err, catalog.ErrGalleryNotInCatalog):
		server.WriteJSON(w, http.StatusNotFound, map[string]string{"error": "gallery_not_found"})
	case errors.Is(err, catalog.ErrInvalidQuery):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_query"})
	case errors.Is(err, catalog.ErrInvalidPagination):
		server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pagination"})
	default:
		server.GetHTTPContext(r).Err = err
		server.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
	}
}
