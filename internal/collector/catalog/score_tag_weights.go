package catalog

import (
	"context"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

// ScoreTagWeights returns, for each element of tags in the same order, whether
// the Panda catalog has the tag, matched by parseScoreTarget with its
// namespace as sent (a short form is not expanded), its effective
// weight under the active model, or under no model while none is active, and
// its pin.
func (s *Service) ScoreTagWeights(ctx context.Context, tags []string) (collectorapi.ScoreTagWeights, error) {
	result := collectorapi.ScoreTagWeights{Tags: make([]collectorapi.ScoreTagWeight, len(tags))}
	// An element that is no namespace:value is sent as NULL, which matches no
	// catalog tag.
	namespaces := make([]*string, len(tags))
	values := make([]*string, len(tags))
	for i, tag := range tags {
		result.Tags[i].Tag = tag
		if target, ok := parseScoreTarget(collectorapi.ScoreKindTag, tag); ok {
			namespaces[i], values[i] = &target.namespace, &target.value
		}
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	filter, err := readScoreFilter(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Active = filter.model != nil
	if len(tags) == 0 {
		return result, nil
	}
	// A NULL model ID gives no calculated weight, so only the pin and the
	// bonus remain.
	var modelID *int64
	if filter.model != nil {
		modelID = &filter.model.id
	}
	rows, err := tx.Query(ctx, `SELECT i.position, t.id IS NOT NULL, `+filter.weightSQL("$3::bigint", collectorapi.ScoreKindTag, "t.id")+`,
			p.id, t.namespace || ':' || t.value, p.weight
		FROM unnest($1::text[], $2::text[]) WITH ORDINALITY AS i (namespace, value_lower, position)
		LEFT JOIN catalog_tags t ON t.namespace = i.namespace AND t.value_lower = i.value_lower
		LEFT JOIN panda_score_pins p ON p.kind = 'tag' AND p.tag_id = t.id`,
		namespaces, values, modelID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			position  int
			known     bool
			weight    *float64
			pinID     *int64
			label     *string
			pinWeight *float64
		)
		if err := rows.Scan(&position, &known, &weight, &pinID, &label, &pinWeight); err != nil {
			return result, err
		}
		entry := &result.Tags[position-1]
		entry.Known, entry.Weight = known, weight
		if pinID != nil {
			entry.Pin = &collectorapi.ScorePin{ID: *pinID, Kind: collectorapi.ScoreKindTag, Label: *label, Weight: *pinWeight}
		}
	}
	return result, rows.Err()
}
