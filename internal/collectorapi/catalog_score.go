package collectorapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// The kinds of value a score override names.
const (
	ScoreKindTag      = "tag"
	ScoreKindUploader = "uploader"
	ScoreKindCategory = "category"
)

// The two rules of a score override.
const (
	ScoreRuleAlwaysHide = "always_hide"
	ScoreRuleNeverHide  = "never_hide"
)

// The sources of an effective weight that the model did not calculate.
const (
	ScoreSourcePin         = "pin"
	ScoreSourceAuthorBonus = "author_bonus"
)

// ScoreRuleBelowThreshold is the rule of a catalog item whose score is below
// the active model's threshold.
const ScoreRuleBelowThreshold = "below_threshold"

// The states of the score filter beside "" (apply it): all galleries, or
// only the galleries it hides.
const (
	ScoreFilterAll    = "all"
	ScoreFilterHidden = "hidden"
)

func ValidScoreFilter(filter string) bool {
	return filter == "" || filter == ScoreFilterAll || filter == ScoreFilterHidden
}

// CatalogScoreFilter describes the score filter in force for a catalog page.
// ThresholdTenths is the active model's threshold, nil without one.
// HiddenUnseen reports, for an empty first page of the unseen view, that the
// filter hides unseen galleries that pass the search and filters.
type CatalogScoreFilter struct {
	ThresholdTenths *int `json:"threshold_tenths,omitempty"`
	HiddenUnseen    bool `json:"hidden_unseen,omitempty"`
}

// CatalogItemScore names the rule that decides a catalog item and the label of
// the deciding override. With an active model
// it also gives the item's score and its strongest weights.
type CatalogItemScore struct {
	Rule     string               `json:"rule,omitempty"`
	Override string               `json:"override,omitempty"`
	Value    *float64             `json:"value,omitempty"`
	Weights  []CatalogScoreWeight `json:"weights,omitempty"`
}

// CatalogScoreWeight is the effective weight of one tag, uploader or category
// of a catalog item under the active model, labelled like an override. Source
// names where the weight comes from when the model did not calculate it.
type CatalogScoreWeight struct {
	Kind   string  `json:"kind"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
	Source string  `json:"source,omitempty"`
}

// ScoreOverride is one stored override. Label is namespace:value for a tag
// and the stored value for an uploader or category.
type ScoreOverride struct {
	ID    int64  `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Rule  string `json:"rule"`
}

// ScorePin is one pinned weight, labelled like an override. ModelWeight is set
// in a preview only: the weight the candidate calculated for the pin's value,
// nil without one.
type ScorePin struct {
	ID          int64    `json:"id"`
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Weight      float64  `json:"weight"`
	ModelWeight *float64 `json:"model_weight,omitempty"`
}

// ScoreNoFavorites is the reason a preview offers no model: no Panda favorite
// is inside the default filter.
const ScoreNoFavorites = "no_favorites"

// ScoreCountsPreparing is the reason a preview offers no model while the
// collector counts the catalog in scope of a new default filter.
const ScoreCountsPreparing = "counts_preparing"

// ScorePreviewOptions asks for a candidate score model from the Panda
// favorites inside the default filter. The default filter describes the
// listing as the user sees it and limits both the favorites and the catalog
// the model is calculated from.
type ScorePreviewOptions struct {
	DefaultQuery      string   `json:"default_query"`
	DefaultCategories []string `json:"default_categories"`
	// CandidateID names the stored candidate to measure again; 0 calculates.
	CandidateID int64 `json:"candidate_id,omitempty"`
}

// ScorePreview is a calculated candidate, or the reason there is none, with
// the numbers of its evidence, the current overrides and the pins, by ID, the
// author bonus (nil while it is off), the number of favorite authors and those
// with the most favorites.
type ScorePreview struct {
	Evidence  ScoreEvidence   `json:"evidence"`
	Candidate *ScoreCandidate `json:"candidate"`
	NoModel   string          `json:"no_model,omitempty"`
	// CountsProgress is the share of the catalog counted so far, in percent,
	// while NoModel is ScoreCountsPreparing.
	CountsProgress *int            `json:"counts_progress,omitempty"`
	Active         *ScoreActive    `json:"active"`
	Overrides      []ScoreOverride `json:"overrides"`
	Pins           []ScorePin      `json:"pins"`
	Recent         ScoreRecent     `json:"recent"`

	AuthorBonus         *float64              `json:"author_bonus"`
	FavoriteAuthorCount int                   `json:"favorite_author_count"`
	FavoriteAuthors     []ScoreFavoriteAuthor `json:"favorite_authors"`
}

// ScoreFavoriteAuthor is one favorite author: its tag as namespace:value,
// the number of Panda favorites that carry it, and its effective weight
// under the candidate, nil without one.
type ScoreFavoriteAuthor struct {
	Label     string   `json:"label"`
	Favorites int      `json:"favorites"`
	Weight    *float64 `json:"weight"`
}

// ScoreTagWeights answers a tag weight lookup. Active says whether a Panda
// score model is active.
type ScoreTagWeights struct {
	Active bool             `json:"active"`
	Tags   []ScoreTagWeight `json:"tags"`
}

// ScoreTagWeight is the answer for one sent tag. Known says whether the Panda
// catalog has the tag. Weight is its effective weight, nil without one.
type ScoreTagWeight struct {
	Tag    string    `json:"tag"`
	Known  bool      `json:"known"`
	Weight *float64  `json:"weight"`
	Pin    *ScorePin `json:"pin,omitempty"`
}

// ScoreActive describes the active model.
type ScoreActive struct {
	CalculatedAt    time.Time `json:"calculated_at"`
	ThresholdTenths int       `json:"threshold_tenths"`
}

// ScoreRecent classes the newest catalog galleries that pass the default
// filter by the first hiding rule that decides them: a "never hide" override,
// an "always hide" override, and for the rest their score under the candidate
// and under the active model. A histogram is nil without its model.
type ScoreRecent struct {
	Total        int             `json:"total"`
	NeverHidden  int             `json:"never_hidden"`
	AlwaysHidden int             `json:"always_hidden"`
	Candidate    *ScoreHistogram `json:"candidate"`
	Active       *ScoreHistogram `json:"active"`
}

// ScoreHistogram counts galleries by score: Counts[i] is the number of
// galleries with floor(score * 10) == FirstBucket + i. A threshold of t tenths
// hides the counts of all buckets below t. The first and the last bucket of a
// histogram that reaches the window's edge (-1000.0 or 1000.0) also count the
// scores beyond it.
type ScoreHistogram struct {
	FirstBucket int   `json:"first_bucket"`
	Counts      []int `json:"counts"`
}

// ScoreEvidence describes the evidence of a preview: the number of Panda
// favorites inside the default filter, the latest sync time of any favorite
// collection, nil without one, and the number of favorites in scope that are
// new since the active model was calculated, nil without one. FewFavorites is
// set while the favorites in scope are under the warning limit.
type ScoreEvidence struct {
	Favorites         int        `json:"favorites"`
	FavoritesSyncedAt *time.Time `json:"favorites_synced_at"`
	NewFavorites      *int       `json:"new_favorites"`
	FewFavorites      bool       `json:"few_favorites"`
}

// ScoreCandidate is the stored candidate model with its strongest weights.
// WeightCount is the number of weights the model has.
type ScoreCandidate struct {
	ID           int64         `json:"id"`
	CalculatedAt time.Time     `json:"calculated_at"`
	WeightCount  int           `json:"weight_count"`
	Weights      []ScoreWeight `json:"weights"`
	// SuggestedThresholdTenths is the start threshold the preview suggests
	// for this candidate.
	SuggestedThresholdTenths int          `json:"suggested_threshold_tenths"`
	HeldOut                  ScoreHeldOut `json:"held_out"`
}

// ScoreHeldOut counts the held-out favorites: those an override always
// hides or never hides, and the scores of the rest. A threshold of t tenths
// hides AlwaysHidden plus the buckets below t.
type ScoreHeldOut struct {
	Total        int            `json:"total"`
	AlwaysHidden int            `json:"always_hidden"`
	NeverHidden  int            `json:"never_hidden"`
	Scores       ScoreHistogram `json:"scores"`
}

// ScoreWeight is the weight of one tag, uploader or category, labelled like an
// override, with the numbers of favorites and of catalog galleries in scope
// that carry it. ActiveWeight is the value's weight in the active model, nil
// without one.
type ScoreWeight struct {
	Kind         string   `json:"kind"`
	Label        string   `json:"label"`
	Weight       float64  `json:"weight"`
	Favorites    int      `json:"favorites"`
	Catalog      int      `json:"catalog"`
	ActiveWeight *float64 `json:"active_weight"`
}

// SetScoreOverride gives the tag, uploader or category the rule, replacing
// its other rule.
func (c *Client) SetScoreOverride(ctx context.Context, kind, value, rule string) error {
	body, err := json.Marshal(struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
		Rule  string `json:"rule"`
	}{kind, value, rule})
	if err != nil {
		return err
	}
	return c.scoreRequest(ctx, http.MethodPost, "/api/catalog/score/overrides", bytes.NewReader(body))
}

// RemoveScoreOverride removes the override with the ID, if it exists.
func (c *Client) RemoveScoreOverride(ctx context.Context, id int64) error {
	return c.scoreRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/catalog/score/overrides/%d", id), nil)
}

// SetScorePin pins the weight of the tag, uploader or category, replacing its
// pinned weight, and returns the stored pin.
func (c *Client) SetScorePin(ctx context.Context, kind, value string, weight float64) (ScorePin, error) {
	body, err := json.Marshal(struct {
		Kind   string  `json:"kind"`
		Value  string  `json:"value"`
		Weight float64 `json:"weight"`
	}{kind, value, weight})
	if err != nil {
		return ScorePin{}, err
	}
	var pin ScorePin
	err = c.catalogJSONRequest(ctx, http.MethodPost, "/api/catalog/score/pins", bytes.NewReader(body), &pin)
	return pin, err
}

// RemoveScorePin removes the pin with the ID, if it exists.
func (c *Client) RemoveScorePin(ctx context.Context, id int64) error {
	return c.scoreRequest(ctx, http.MethodDelete, fmt.Sprintf("/api/catalog/score/pins/%d", id), nil)
}

// SetAuthorBonus sets the author bonus, a number above 0; nil turns it off.
func (c *Client) SetAuthorBonus(ctx context.Context, bonus *float64) error {
	body, err := json.Marshal(struct {
		Bonus *float64 `json:"bonus"`
	}{bonus})
	if err != nil {
		return err
	}
	return c.scoreRequest(ctx, http.MethodPut, "/api/catalog/score/author-bonus", bytes.NewReader(body))
}

// ScoreTagWeights returns the effective weight and the pin of each tag, in the
// sent order.
func (c *Client) ScoreTagWeights(ctx context.Context, tags []string) (ScoreTagWeights, error) {
	if tags == nil {
		tags = []string{}
	}
	body, err := json.Marshal(struct {
		Tags []string `json:"tags"`
	}{tags})
	if err != nil {
		return ScoreTagWeights{}, err
	}
	var weights ScoreTagWeights
	if err := c.catalogJSONRequest(ctx, http.MethodPost, "/api/catalog/score/tags", bytes.NewReader(body), &weights); err != nil {
		return ScoreTagWeights{}, err
	}
	if len(weights.Tags) != len(tags) {
		return ScoreTagWeights{}, fmt.Errorf("tag weights: %d entries for %d tags", len(weights.Tags), len(tags))
	}
	return weights, nil
}

// PreviewScoreModel calculates a candidate score model and returns it with
// its evidence and effect.
func (c *Client) PreviewScoreModel(ctx context.Context, options ScorePreviewOptions) (ScorePreview, error) {
	body, err := json.Marshal(options)
	if err != nil {
		return ScorePreview{}, err
	}
	var preview ScorePreview
	err = c.catalogJSONRequest(ctx, http.MethodPost, "/api/catalog/score/preview", bytes.NewReader(body), &preview)
	return preview, err
}

// ApplyScoreModel makes the candidate the active model with the threshold.
func (c *Client) ApplyScoreModel(ctx context.Context, candidateID int64, thresholdTenths int) error {
	body, err := json.Marshal(struct {
		CandidateID     int64 `json:"candidate_id"`
		ThresholdTenths int   `json:"threshold_tenths"`
	}{candidateID, thresholdTenths})
	if err != nil {
		return err
	}
	return c.scoreRequest(ctx, http.MethodPost, "/api/catalog/score/apply", bytes.NewReader(body))
}

// SetScoreThreshold changes the active model's threshold.
func (c *Client) SetScoreThreshold(ctx context.Context, thresholdTenths int) error {
	body, err := json.Marshal(struct {
		ThresholdTenths int `json:"threshold_tenths"`
	}{thresholdTenths})
	if err != nil {
		return err
	}
	return c.scoreRequest(ctx, http.MethodPut, "/api/catalog/score/threshold", bytes.NewReader(body))
}

// RemoveScoreModel turns the active model off, if there is one.
func (c *Client) RemoveScoreModel(ctx context.Context) error {
	return c.scoreRequest(ctx, http.MethodDelete, "/api/catalog/score/model", nil)
}

func (c *Client) scoreRequest(ctx context.Context, method, path string, body io.Reader) error {
	response, err := c.controlRequest(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return downloadResponseError(response)
	}
	return nil
}
