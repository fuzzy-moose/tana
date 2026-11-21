package httpapi

import (
	"net/http"
	"strconv"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/catalogfilter"
	"github.com/fuzzy-moose/tana/internal/server"
)

func HandleCollectorSetScoreOverride(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			Kind  string `json:"kind"`
			Value string `json:"value"`
			Rule  string `json:"rule"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.SetScoreOverride(r.Context(), input.Kind, input.Value, input.Rule); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorRemoveScoreOverride(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		// An ID that is not a positive integer is an invalid override, not
		// the 404 of pathID.
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_override"})
			return
		}
		if err := client.RemoveScoreOverride(r.Context(), id); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorSetScorePin(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			Kind   string   `json:"kind"`
			Value  string   `json:"value"`
			Weight *float64 `json:"weight"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if input.Weight == nil {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pin"})
			return
		}
		pin, err := client.SetScorePin(r.Context(), input.Kind, input.Value, *input.Weight)
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, pin)
	})
}

func HandleCollectorRemoveScorePin(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		// An ID that is not a positive integer is an invalid pin, not the 404
		// of pathID.
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			server.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_pin"})
			return
		}
		if err := client.RemoveScorePin(r.Context(), id); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// HandleCollectorScorePreview always sends the default filter: the preview
// describes the listing as the user sees it, so it has no bypass. The body
// names at most the candidate to measure again.
func HandleCollectorScorePreview(client *collectorapi.Client, defaults *catalogfilter.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			CandidateID int64 `json:"candidate_id"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		options := collectorapi.ScorePreviewOptions{DefaultCategories: []string{}, CandidateID: input.CandidateID}
		if defaults != nil {
			filter, err := defaults.Load(r.Context())
			if err != nil {
				writeDefaultFilterError(w, r, err)
				return
			}
			options.DefaultQuery = filter.Query
			if filter.Categories != nil {
				options.DefaultCategories = filter.Categories
			}
		}
		preview, err := client.PreviewScoreModel(r.Context(), options)
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, preview)
	})
}

func HandleCollectorApplyScoreModel(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			CandidateID     int64 `json:"candidate_id"`
			ThresholdTenths int   `json:"threshold_tenths"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.ApplyScoreModel(r.Context(), input.CandidateID, input.ThresholdTenths); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorSetScoreThreshold(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			ThresholdTenths int `json:"threshold_tenths"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.SetScoreThreshold(r.Context(), input.ThresholdTenths); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorSetAuthorBonus(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			Bonus *float64 `json:"bonus"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		if err := client.SetAuthorBonus(r.Context(), input.Bonus); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func HandleCollectorScoreTagWeights(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		var input struct {
			Tags []string `json:"tags"`
		}
		if !server.DecodeJSON(w, r, &input) {
			return
		}
		weights, err := client.ScoreTagWeights(r.Context(), input.Tags)
		if err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		server.WriteJSON(w, http.StatusOK, weights)
	})
}

func HandleCollectorRemoveScoreModel(client *collectorapi.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !catalogCollectorConfigured(w, client) {
			return
		}
		if err := client.RemoveScoreModel(r.Context()); err != nil {
			writeCatalogCollectorError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
