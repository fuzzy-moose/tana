package catalog

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

var ErrInvalidScorePin = errors.New("invalid score pin")

// The largest magnitude of a pin and of the author bonus. The sum of a
// gallery's weights, times ten, then stays a finite float.
const maxScoreWeight = 1e300

// scorePin is a stored pin with the catalog tag it names, if any.
type scorePin struct {
	collectorapi.ScorePin
	TagID int64
}

// SetScorePin pins the weight of one tag, uploader or category, with the value
// rules of SetScoreOverride, and replaces the weight of an existing pin of the
// same value. It returns the stored pin. Pins belong to no model, so it takes
// no lock on panda_score_models.
func (s *Service) SetScorePin(ctx context.Context, kind, value string, weight float64) (collectorapi.ScorePin, error) {
	if math.IsNaN(weight) || math.Abs(weight) > maxScoreWeight {
		return collectorapi.ScorePin{}, ErrInvalidScorePin
	}
	target, ok := parseScoreTarget(kind, value)
	if !ok {
		return collectorapi.ScorePin{}, ErrInvalidScorePin
	}
	pin := collectorapi.ScorePin{Kind: target.kind}
	if target.kind == collectorapi.ScoreKindTag {
		err := s.db.QueryRow(ctx, `WITH tag AS (
				SELECT id, namespace || ':' || value AS label FROM catalog_tags WHERE namespace = $1 AND value_lower = $2
			), pin AS (
				INSERT INTO panda_score_pins (kind, tag_id, weight, created_at)
				SELECT 'tag', id, $3, now() FROM tag
				ON CONFLICT (tag_id) WHERE kind = 'tag' DO UPDATE SET weight = excluded.weight
				RETURNING id, weight
			)
			SELECT pin.id, tag.label, pin.weight FROM pin, tag`,
			target.namespace, target.value, weight).Scan(&pin.ID, &pin.Label, &pin.Weight)
		if errors.Is(err, pgx.ErrNoRows) {
			return collectorapi.ScorePin{}, ErrTagNotFound
		}
		return pin, err
	}
	err := s.db.QueryRow(ctx, `INSERT INTO panda_score_pins (kind, value, weight, created_at) VALUES ($1, $2, $3, now())
		ON CONFLICT (kind, value) WHERE kind <> 'tag' DO UPDATE SET weight = excluded.weight
		RETURNING id, value, weight`, target.kind, target.value, weight).Scan(&pin.ID, &pin.Label, &pin.Weight)
	return pin, err
}

// RemoveScorePin deletes the pin with the ID, if it exists.
func (s *Service) RemoveScorePin(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalidScorePin
	}
	_, err := s.db.Exec(ctx, "DELETE FROM panda_score_pins WHERE id = $1", id)
	return err
}

// scorePins reads every pin with its label, namespace:value in the stored
// spelling of the tag or the stored value, ordered by ID.
func scorePins(ctx context.Context, tx pgx.Tx) ([]scorePin, error) {
	rows, err := tx.Query(ctx, `SELECT p.id, p.kind, COALESCE(p.tag_id, 0),
			CASE WHEN p.kind = 'tag' THEN t.namespace || ':' || t.value ELSE p.value END, p.weight
		FROM panda_score_pins p LEFT JOIN catalog_tags t ON t.id = p.tag_id
		ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pins []scorePin
	for rows.Next() {
		var pin scorePin
		if err := rows.Scan(&pin.ID, &pin.Kind, &pin.TagID, &pin.Label, &pin.Weight); err != nil {
			return nil, err
		}
		pins = append(pins, pin)
	}
	return pins, rows.Err()
}

// pinnedWeights maps each pinned value to its pin.
func pinnedWeights(pins []scorePin) map[scoreValue]float64 {
	weights := make(map[scoreValue]float64, len(pins))
	for _, pin := range pins {
		value := scoreValue{kind: pin.Kind, tagID: pin.TagID}
		if pin.Kind != collectorapi.ScoreKindTag {
			value.value = pin.Label
		}
		weights[value] = pin.Weight
	}
	return weights
}
