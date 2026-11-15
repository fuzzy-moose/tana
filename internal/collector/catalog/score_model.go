package catalog

import (
	"cmp"
	"context"
	"math"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// scorePreviewWeights is the number of the strongest weights a preview returns.
const scorePreviewWeights = 500

// scorePreviewAuthors is the number of the favorite authors with the most
// favorites that a preview returns.
const scorePreviewAuthors = 500

// PreviewScoreModel calculates a candidate score model from the evidence and
// stores it as the one candidate, replacing every other model that is not
// active. Without a model it stores none and names the side that is missing.
func (s *Service) PreviewScoreModel(ctx context.Context, options collectorapi.ScorePreviewOptions) (collectorapi.ScorePreview, error) {
	var preview collectorapi.ScorePreview
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return preview, err
	}
	defer tx.Rollback(ctx)
	// Previews run one after another, so that only the newest candidate
	// remains, and the active model they compare with stays as read.
	if _, err := tx.Exec(ctx, "LOCK TABLE panda_score_models IN SHARE ROW EXCLUSIVE MODE"); err != nil {
		return preview, err
	}
	filter, err := readScoreFilter(ctx, tx)
	if err != nil {
		return preview, err
	}
	active := filter.model
	switch {
	case options.Favorites != nil:
		preview.Favorites = *options.Favorites
	case active != nil:
		preview.Favorites = active.favorites
	}
	predicate, predicateArgs, pageCategories, err := catalogPredicate(ctx, tx, collectorapi.CatalogOptions{
		DefaultQuery: options.DefaultQuery, DefaultCategories: options.DefaultCategories,
	})
	if err != nil {
		return preview, err
	}
	evidence, err := readEvidence(ctx, tx, preview.Favorites)
	if err != nil {
		return preview, err
	}
	for _, gallery := range evidence {
		switch {
		case gallery.class == scoreNotForMe:
			preview.Evidence.NotForMe++
		case gallery.class == scoreNeutral:
			preview.Evidence.Neutral++
		case gallery.favorite:
			preview.Evidence.CountedFavorites++
		default:
			preview.Evidence.Interesting++
		}
	}
	preview.Evidence.FewVerdicts = preview.Evidence.Interesting < FewVerdicts || preview.Evidence.NotForMe < FewVerdicts
	if err := tx.QueryRow(ctx, "SELECT max(synced_at) FROM favorite_categories").Scan(&preview.Evidence.FavoritesSyncedAt); err != nil {
		return preview, err
	}
	preview.Evidence.FavoritesSyncedAt = pgdb.UTC(preview.Evidence.FavoritesSyncedAt)
	preview.Overrides = make([]collectorapi.ScoreOverride, 0, len(filter.overrides))
	for _, override := range filter.overrides {
		preview.Overrides = append(preview.Overrides, override.ScoreOverride)
	}
	slices.SortFunc(preview.Overrides, func(a, b collectorapi.ScoreOverride) int { return cmp.Compare(a.ID, b.ID) })
	preview.Pins = make([]collectorapi.ScorePin, 0, len(filter.pins))
	for _, pin := range filter.pins {
		preview.Pins = append(preview.Pins, pin.ScorePin)
	}
	preview.AuthorBonus = filter.authorBonus
	authors, err := favoriteAuthors(ctx, tx)
	if err != nil {
		return preview, err
	}
	rule := &weightRule{pins: pinnedWeights(filter.pins), authorBonus: filter.authorBonus, favoriteAuthors: make(map[int64]bool, len(authors))}
	for _, author := range authors {
		rule.favoriteAuthors[author.tagID] = true
	}
	var activeWeights map[scoreValue]float64
	if active != nil {
		preview.Active = &collectorapi.ScoreActive{
			CalculatedAt: active.calculatedAt.UTC(), Favorites: active.favorites, ThresholdTenths: active.thresholdTenths,
		}
		var newVerdicts int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM panda_gallery_verdicts v
			JOIN catalog_galleries g ON g.gallery_id = v.gallery_id WHERE v.given_at > $1`, active.calculatedAt).Scan(&newVerdicts); err != nil {
			return preview, err
		}
		preview.Evidence.NewVerdicts = &newVerdicts
		if activeWeights, err = modelWeights(ctx, tx, active.id); err != nil {
			return preview, err
		}
	}
	if _, err := tx.Exec(ctx, "DELETE FROM panda_score_models WHERE NOT active"); err != nil {
		return preview, err
	}
	galleries := make([]evidenceGallery, 0, len(evidence))
	for _, gallery := range evidence {
		galleries = append(galleries, gallery.evidenceGallery)
	}
	total, values := tallyScoreCounts(galleries)
	weights, ok := scoreWeights(total, values)
	switch {
	case preview.Evidence.Interesting+preview.Evidence.NotForMe == 0:
		preview.NoModel = collectorapi.ScoreNoVerdicts
	case !ok && total.notForMe == 0:
		preview.NoModel = collectorapi.ScoreNoNotForMe
	case !ok:
		preview.NoModel = collectorapi.ScoreNoAcceptable
	default:
		candidate, err := storeCandidate(ctx, tx, preview.Favorites, weights, activeWeights, values)
		if err != nil {
			return preview, err
		}
		tenths := heldOutTenths(galleries, total, values, rule)
		if candidate.HeldOut, err = filter.heldOut(ctx, tx, predicate, predicateArgs, galleries, tenths); err != nil {
			return preview, err
		}
		candidate.SuggestedThresholdTenths = suggestedThreshold(candidate.HeldOut.Interesting)
		preview.Candidate = &candidate
	}
	var candidateWeights map[scoreValue]float64
	if preview.Candidate != nil {
		candidateWeights = weights
	}
	// All weights of the candidate, not only those it lists: a pin is
	// typically set on a value with a weak weight.
	for i, pin := range filter.pins {
		if weight, ok := candidateWeights[pin.value()]; ok {
			preview.Pins[i].ModelWeight = &weight
		}
	}
	preview.FavoriteAuthorCount = len(authors)
	preview.FavoriteAuthors = make([]collectorapi.ScoreFavoriteAuthor, 0, min(len(authors), scorePreviewAuthors))
	for _, author := range authors[:min(len(authors), scorePreviewAuthors)] {
		listed := collectorapi.ScoreFavoriteAuthor{Label: author.label, Favorites: author.favorites}
		weight, ok := rule.effective(scoreValue{kind: collectorapi.ScoreKindTag, tagID: author.tagID}, func(value scoreValue) (float64, bool) {
			weight, ok := candidateWeights[value]
			return weight, ok
		})
		if ok {
			listed.Weight = &weight
		}
		preview.FavoriteAuthors = append(preview.FavoriteAuthors, listed)
	}
	var candidateID *int64
	if preview.Candidate != nil {
		candidateID = &preview.Candidate.ID
	}
	if preview.Recent, err = filter.recent(ctx, tx, predicate, predicateArgs, pageCategories, candidateID); err != nil {
		return preview, err
	}
	return preview, tx.Commit(ctx)
}

// modelWeights reads every weight of the model.
func modelWeights(ctx context.Context, tx pgx.Tx, modelID int64) (map[scoreValue]float64, error) {
	rows, err := tx.Query(ctx, "SELECT kind, COALESCE(tag_id, 0), value, weight FROM panda_score_weights WHERE model_id = $1", modelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	weights := map[scoreValue]float64{}
	for rows.Next() {
		var value scoreValue
		var weight float64
		if err := rows.Scan(&value.kind, &value.tagID, &value.value, &weight); err != nil {
			return nil, err
		}
		weights[value] = weight
	}
	return weights, rows.Err()
}

// favoriteAuthor is a row of favorite_authors with the label of its tag.
type favoriteAuthor struct {
	tagID     int64
	label     string
	favorites int
}

// favoriteAuthors reads every favorite author with namespace:value in the
// tag's stored spelling, more favorites first, then by label.
func favoriteAuthors(ctx context.Context, tx pgx.Tx) ([]favoriteAuthor, error) {
	rows, err := tx.Query(ctx, `SELECT fa.tag_id, t.namespace || ':' || t.value, fa.favorites
		FROM favorite_authors fa JOIN catalog_tags t ON t.id = fa.tag_id`)
	if err != nil {
		return nil, err
	}
	authors, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (favoriteAuthor, error) {
		var author favoriteAuthor
		err := row.Scan(&author.tagID, &author.label, &author.favorites)
		return author, err
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(authors, func(a, b favoriteAuthor) int {
		return cmp.Or(cmp.Compare(b.favorites, a.favorites), strings.Compare(a.label, b.label))
	})
	return authors, nil
}

// classifiedGallery is an evidence gallery that also says whether it counts
// as "Interesting" because it is a favorite.
type classifiedGallery struct {
	evidenceGallery
	favorite bool
}

// readEvidence reads every evidence gallery once, with its class: its
// verdict; else, with favorites, "Interesting" when it is in any favorite
// collection; else neutral when a display of it is evidence. A gallery without
// a catalog entry gives no evidence.
func readEvidence(ctx context.Context, tx pgx.Tx, favorites bool) ([]classifiedGallery, error) {
	rows, err := tx.Query(ctx, `WITH favorite AS (
			SELECT DISTINCT gallery_id FROM favorites WHERE $1::boolean
		), candidate AS (
			SELECT gallery_id FROM panda_gallery_verdicts
			UNION SELECT gallery_id FROM favorite
			UNION SELECT gallery_id FROM panda_gallery_displays WHERE evidence
		)
		SELECT g.gallery_id, COALESCE(v.verdict, ''), f.gallery_id IS NOT NULL, g.uploader, g.category,
			ARRAY(SELECT gt.tag_id FROM catalog_gallery_tags gt WHERE gt.gallery_id = g.gallery_id)
		FROM candidate c
		JOIN catalog_galleries g ON g.gallery_id = c.gallery_id
		LEFT JOIN panda_gallery_verdicts v ON v.gallery_id = c.gallery_id
		LEFT JOIN favorite f ON f.gallery_id = c.gallery_id
		ORDER BY g.gallery_id`, favorites)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var evidence []classifiedGallery
	for rows.Next() {
		var gallery classifiedGallery
		var verdict string
		var favorite bool
		if err := rows.Scan(&gallery.galleryID, &verdict, &favorite, &gallery.uploader, &gallery.category, &gallery.tagIDs); err != nil {
			return nil, err
		}
		switch {
		case verdict == collectorapi.VerdictNotForMe:
			gallery.class = scoreNotForMe
		case verdict == collectorapi.VerdictInteresting:
			gallery.class = scoreInteresting
		case favorite:
			gallery.class = scoreInteresting
			gallery.favorite = true
		default:
			gallery.class = scoreNeutral
		}
		evidence = append(evidence, gallery)
	}
	return evidence, rows.Err()
}

// storeCandidate stores the model and all its weights in two statements and
// returns it with its strongest weights, each with its weight in the active
// model when that has one.
func storeCandidate(ctx context.Context, tx pgx.Tx, favorites bool, weights, activeWeights map[scoreValue]float64, values map[scoreValue]scoreCounts) (collectorapi.ScoreCandidate, error) {
	candidate := collectorapi.ScoreCandidate{Favorites: favorites, WeightCount: len(weights)}
	if err := tx.QueryRow(ctx, `INSERT INTO panda_score_models (favorites, calculated_at) VALUES ($1, now())
		RETURNING id, calculated_at`, favorites).Scan(&candidate.ID, &candidate.CalculatedAt); err != nil {
		return candidate, err
	}
	candidate.CalculatedAt = candidate.CalculatedAt.UTC()
	labels, err := tagLabels(ctx, tx, weights)
	if err != nil {
		return candidate, err
	}
	rows := make([]collectorapi.ScoreWeight, 0, len(weights))
	var kinds, texts []string
	var tagIDs []int64
	var weightValues []float64
	var interesting, notForMe, neutral []int32
	for value, weight := range weights {
		counts := values[value]
		label := value.value
		if value.kind == collectorapi.ScoreKindTag {
			label = labels[value.tagID]
		}
		row := collectorapi.ScoreWeight{Kind: value.kind, Label: label, Weight: weight,
			Interesting: counts.interesting, NotForMe: counts.notForMe, Neutral: counts.neutral}
		if activeWeight, ok := activeWeights[value]; ok {
			row.ActiveWeight = &activeWeight
		}
		rows = append(rows, row)
		kinds = append(kinds, value.kind)
		texts = append(texts, value.value)
		tagIDs = append(tagIDs, value.tagID)
		weightValues = append(weightValues, weight)
		interesting = append(interesting, int32(counts.interesting))
		notForMe = append(notForMe, int32(counts.notForMe))
		neutral = append(neutral, int32(counts.neutral))
	}
	if _, err := tx.Exec(ctx, `INSERT INTO panda_score_weights (model_id, kind, tag_id, value, weight, interesting, not_for_me, neutral)
		SELECT $1, w.kind, NULLIF(w.tag_id, 0), w.value, w.weight, w.interesting, w.not_for_me, w.neutral
		FROM unnest($2::text[], $3::bigint[], $4::text[], $5::float8[], $6::int[], $7::int[], $8::int[])
			AS w(kind, tag_id, value, weight, interesting, not_for_me, neutral)`,
		candidate.ID, kinds, tagIDs, texts, weightValues, interesting, notForMe, neutral); err != nil {
		return candidate, err
	}
	slices.SortFunc(rows, func(a, b collectorapi.ScoreWeight) int {
		return cmp.Or(cmp.Compare(math.Abs(b.Weight), math.Abs(a.Weight)),
			cmp.Compare(scoreKindOrder[a.Kind], scoreKindOrder[b.Kind]), strings.Compare(a.Label, b.Label))
	})
	candidate.Weights = rows[:min(len(rows), scorePreviewWeights)]
	return candidate, nil
}

// tagLabels reads namespace:value, in the stored spelling, for each weighted
// tag.
func tagLabels(ctx context.Context, tx pgx.Tx, weights map[scoreValue]float64) (map[int64]string, error) {
	var ids []int64
	for value := range weights {
		if value.kind == collectorapi.ScoreKindTag {
			ids = append(ids, value.tagID)
		}
	}
	rows, err := tx.Query(ctx, "SELECT id, namespace || ':' || value FROM catalog_tags WHERE id = ANY($1::bigint[])", ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := make(map[int64]string, len(ids))
	for rows.Next() {
		var id int64
		var label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, err
		}
		labels[id] = label
	}
	return labels, rows.Err()
}
