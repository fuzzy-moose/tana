package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var (
	ErrInvalidScoreOverride = errors.New("invalid score override")
	ErrTagNotFound          = errors.New("tag not found")
)

// scoreOverride is a stored override with the catalog tag it names, if any.
type scoreOverride struct {
	collectorapi.ScoreOverride
	TagID int64
}

// SetScoreOverride stores the rule for one tag, uploader or category, and
// replaces the other rule of the same value. A tag must be in catalog_tags,
// matched with the folding of replaceTags; an uploader is kept as given.
func (s *Service) SetScoreOverride(ctx context.Context, kind, value, rule string) error {
	if rule != collectorapi.ScoreRuleAlwaysHide && rule != collectorapi.ScoreRuleNeverHide {
		return ErrInvalidScoreOverride
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ErrInvalidScoreOverride
	}
	switch kind {
	case collectorapi.ScoreKindTag:
		namespace, tag, found := strings.Cut(value, ":")
		namespace, tag = strings.TrimSpace(namespace), strings.TrimSpace(tag)
		if !found || namespace == "" || tag == "" {
			return ErrInvalidScoreOverride
		}
		result, err := s.db.Exec(ctx, `INSERT INTO panda_score_overrides (kind, tag_id, rule, created_at)
			SELECT 'tag', id, $3, now() FROM catalog_tags WHERE namespace = $1 AND value_lower = $2
			ON CONFLICT (tag_id) WHERE kind = 'tag' DO UPDATE SET rule = excluded.rule`,
			strings.ToLower(namespace), strings.ToLower(strings.ReplaceAll(tag, "_", " ")), rule)
		if err != nil {
			return err
		}
		if result.RowsAffected() == 0 {
			return ErrTagNotFound
		}
		return nil
	case collectorapi.ScoreKindCategory:
		categories, err := panda.NormalizeCategories([]string{value})
		if err != nil {
			return ErrInvalidScoreOverride
		}
		value = categories[0]
	case collectorapi.ScoreKindUploader:
	default:
		return ErrInvalidScoreOverride
	}
	_, err := s.db.Exec(ctx, `INSERT INTO panda_score_overrides (kind, value, rule, created_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (kind, value) WHERE kind <> 'tag' DO UPDATE SET rule = excluded.rule`, kind, value, rule)
	return err
}

// RemoveScoreOverride deletes the override with the ID, if it exists.
func (s *Service) RemoveScoreOverride(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalidScoreOverride
	}
	_, err := s.db.Exec(ctx, "DELETE FROM panda_score_overrides WHERE id = $1", id)
	return err
}

// scoreOverrides reads every override with its label: namespace:value in the
// stored spelling of the tag, or the stored value.
func scoreOverrides(ctx context.Context, tx pgx.Tx) ([]scoreOverride, error) {
	rows, err := tx.Query(ctx, `SELECT o.id, o.kind, o.rule, COALESCE(o.tag_id, 0),
			CASE WHEN o.kind = 'tag' THEN t.namespace || ':' || t.value ELSE o.value END
		FROM panda_score_overrides o LEFT JOIN catalog_tags t ON t.id = o.tag_id
		ORDER BY o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var overrides []scoreOverride
	for rows.Next() {
		var override scoreOverride
		if err := rows.Scan(&override.ID, &override.Kind, &override.Rule, &override.TagID, &override.Label); err != nil {
			return nil, err
		}
		overrides = append(overrides, override)
	}
	return overrides, rows.Err()
}
