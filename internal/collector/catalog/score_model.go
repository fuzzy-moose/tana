package catalog

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// scorePreviewWeights is the number of the strongest weights a preview returns.
const scorePreviewWeights = 500

// scorePreviewAuthors is the number of the favorite authors with the most
// favorites that a preview returns.
const scorePreviewAuthors = 500

// PreviewScoreModel calculates a candidate score model from the Panda
// favorites inside the default filter, against the Panda catalog under the
// same filter, and stores it as the one candidate, replacing every other model
// that is not active. Without a favorite in scope it stores none. With the
// CandidateID of the stored candidate it calculates nothing and measures that
// candidate again with the current favorites, pins, overrides and bonus.
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
	defaults := collectorapi.CatalogOptions{DefaultQuery: options.DefaultQuery, DefaultCategories: options.DefaultCategories}
	// The recent sample is what the listing shows; the scope of the
	// calculation also holds expunged galleries, about which the default
	// filter says nothing.
	predicate, predicateArgs, pageCategories, err := catalogPredicate(ctx, tx, defaults)
	if err != nil {
		return preview, err
	}
	defaults.IncludeExpunged = true
	scope, scopeArgs, _, err := catalogPredicate(ctx, tx, defaults)
	if err != nil {
		return preview, err
	}
	favorites, err := favoritesInScope(ctx, tx, scope, scopeArgs)
	if err != nil {
		return preview, err
	}
	preview.Evidence.Favorites = len(favorites)
	preview.Evidence.FewFavorites = len(favorites) < FewFavorites
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
			CalculatedAt: active.calculatedAt.UTC(), ThresholdTenths: active.thresholdTenths,
		}
		if activeWeights, err = modelWeights(ctx, tx, active.id); err != nil {
			return preview, err
		}
		if preview.Evidence.NewFavorites, err = newFavorites(ctx, tx, favorites, active.calculatedAt); err != nil {
			return preview, err
		}
	}
	var calculation scoreCalculation
	if options.CandidateID != 0 {
		// A measurement keeps the stored candidate as calculated.
		stored, calculatedAt, ok, err := storedCalculation(ctx, tx, options.CandidateID)
		if err != nil {
			return preview, err
		}
		if ok {
			calculation = stored
			preview.Candidate = &collectorapi.ScoreCandidate{ID: options.CandidateID, CalculatedAt: calculatedAt.UTC()}
		}
	}
	if preview.Candidate == nil {
		if _, err := tx.Exec(ctx, "DELETE FROM panda_score_models WHERE NOT active"); err != nil {
			return preview, err
		}
		if len(favorites) == 0 {
			preview.NoModel = collectorapi.ScoreNoFavorites
		} else {
			if calculation, err = calculateScoreModel(ctx, tx, scope, scopeArgs, favorites); err != nil {
				return preview, err
			}
			candidate, err := storeCandidate(ctx, tx, calculation)
			if err != nil {
				return preview, err
			}
			preview.Candidate = &candidate
		}
	}
	var candidateWeights map[scoreValue]float64
	if preview.Candidate != nil {
		candidate := preview.Candidate
		candidate.WeightCount = len(calculation.weights)
		if candidate.Weights, err = listedWeights(ctx, tx, calculation, activeWeights); err != nil {
			return preview, err
		}
		candidate.HeldOut = filter.heldOut(favorites, calculation, authors)
		candidate.SuggestedThresholdTenths = suggestedThreshold(candidate.HeldOut)
		candidateWeights = calculation.weights
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

// favoriteGallery is a favorite in scope with its catalog values.
type favoriteGallery struct {
	galleryID int64
	uploader  string
	category  string
	tagIDs    []int64
}

// values lists the values the gallery carries: its tags, its uploader and its
// category, the last two only when known.
func (g favoriteGallery) values() []scoreValue {
	values := make([]scoreValue, 0, len(g.tagIDs)+2)
	for _, tagID := range g.tagIDs {
		values = append(values, scoreValue{kind: collectorapi.ScoreKindTag, tagID: tagID})
	}
	if g.uploader != "" {
		values = append(values, scoreValue{kind: collectorapi.ScoreKindUploader, value: g.uploader})
	}
	if g.category != "" {
		values = append(values, scoreValue{kind: collectorapi.ScoreKindCategory, value: g.category})
	}
	return values
}

// favoritesInScope reads once each gallery that is a favorite in any
// collection and whose catalog entry passes the scope, with its values.
func favoritesInScope(ctx context.Context, tx pgx.Tx, scope string, args pgdb.Args) ([]favoriteGallery, error) {
	rows, err := tx.Query(ctx, `SELECT g.gallery_id, g.uploader, g.category,
			ARRAY(SELECT gt.tag_id FROM catalog_gallery_tags gt WHERE gt.gallery_id = g.gallery_id)
		FROM catalog_galleries g
		WHERE g.gallery_id IN (SELECT gallery_id FROM favorites) AND (`+scope+`)
		ORDER BY g.gallery_id`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (favoriteGallery, error) {
		var gallery favoriteGallery
		err := row.Scan(&gallery.galleryID, &gallery.uploader, &gallery.category, &gallery.tagIDs)
		return gallery, err
	})
}

// newFavorites counts the favorites in scope that the collector first
// observed after the time.
func newFavorites(ctx context.Context, tx pgx.Tx, favorites []favoriteGallery, since time.Time) (*int, error) {
	ids := make([]int64, len(favorites))
	for i, gallery := range favorites {
		ids[i] = gallery.galleryID
	}
	var count int
	err := tx.QueryRow(ctx, "SELECT count(*) FROM favorite_observations WHERE gallery_id = ANY($1::bigint[]) AND observed_at > $2",
		ids, since).Scan(&count)
	return &count, err
}

// scoreCalculation is a calculated model with what it was calculated from:
// the numbers of favorites and catalog galleries in scope, and for each
// weighted value the numbers of those that carry it.
type scoreCalculation struct {
	favoritesInScope, catalogInScope int
	weights                          map[scoreValue]float64
	favorites, catalog               map[scoreValue]int
}

// calculateScoreModel weighs every value that at least MinSupport favorites
// carry against its frequency in the catalog in scope. The catalog is
// counted in the database, and only for those values.
func calculateScoreModel(ctx context.Context, tx pgx.Tx, scope string, args pgdb.Args, favorites []favoriteGallery) (scoreCalculation, error) {
	calculation := scoreCalculation{favoritesInScope: len(favorites), weights: map[scoreValue]float64{}, catalog: map[scoreValue]int{}}
	carried := map[scoreValue]int{}
	for _, gallery := range favorites {
		for _, value := range gallery.values() {
			carried[value]++
		}
	}
	var tagIDs []int64
	var uploaders, categories []string
	for value, count := range carried {
		if count < MinSupport {
			delete(carried, value)
			continue
		}
		switch value.kind {
		case collectorapi.ScoreKindTag:
			tagIDs = append(tagIDs, value.tagID)
		case collectorapi.ScoreKindUploader:
			uploaders = append(uploaders, value.value)
		case collectorapi.ScoreKindCategory:
			categories = append(categories, value.value)
		}
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM catalog_galleries g WHERE "+scope, args...).Scan(&calculation.catalogInScope); err != nil {
		return calculation, err
	}
	counted := slices.Clone(args)
	tagsArg, uploadersArg, categoriesArg := counted.Add(tagIDs), counted.Add(uploaders), counted.Add(categories)
	rows, err := tx.Query(ctx, `SELECT 'tag', gt.tag_id, '', count(*)
		FROM catalog_gallery_tags gt JOIN catalog_galleries g ON g.gallery_id = gt.gallery_id
		WHERE gt.tag_id = ANY(`+tagsArg+`::bigint[]) AND (`+scope+`) GROUP BY gt.tag_id
		UNION ALL
		SELECT 'uploader', 0, g.uploader, count(*) FROM catalog_galleries g
		WHERE g.uploader = ANY(`+uploadersArg+`::text[]) AND (`+scope+`) GROUP BY g.uploader
		UNION ALL
		SELECT 'category', 0, g.category, count(*) FROM catalog_galleries g
		WHERE g.category = ANY(`+categoriesArg+`::text[]) AND (`+scope+`) GROUP BY g.category`, counted...)
	if err != nil {
		return calculation, err
	}
	defer rows.Close()
	for rows.Next() {
		var value scoreValue
		var count int
		if err := rows.Scan(&value.kind, &value.tagID, &value.value, &count); err != nil {
			return calculation, err
		}
		calculation.catalog[value] = count
	}
	if err := rows.Err(); err != nil {
		return calculation, err
	}
	calculation.favorites = carried
	for value, count := range carried {
		if weight, ok := scoreWeight(count, calculation.favoritesInScope, calculation.catalog[value], calculation.catalogInScope); ok {
			calculation.weights[value] = weight
		}
	}
	return calculation, nil
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

// storeCandidate stores the model with the numbers it was calculated from
// and all its weights in two statements, and returns its ID and calculation
// time.
func storeCandidate(ctx context.Context, tx pgx.Tx, calculation scoreCalculation) (collectorapi.ScoreCandidate, error) {
	var candidate collectorapi.ScoreCandidate
	if err := tx.QueryRow(ctx, `INSERT INTO panda_score_models (calculated_at, favorites_in_scope, catalog_in_scope)
		VALUES (now(), $1, $2) RETURNING id, calculated_at`,
		calculation.favoritesInScope, calculation.catalogInScope).Scan(&candidate.ID, &candidate.CalculatedAt); err != nil {
		return candidate, err
	}
	candidate.CalculatedAt = candidate.CalculatedAt.UTC()
	var kinds, texts []string
	var tagIDs []int64
	var weightValues []float64
	var favorites, catalog []int32
	for value, weight := range calculation.weights {
		kinds = append(kinds, value.kind)
		texts = append(texts, value.value)
		tagIDs = append(tagIDs, value.tagID)
		weightValues = append(weightValues, weight)
		favorites = append(favorites, int32(calculation.favorites[value]))
		catalog = append(catalog, int32(calculation.catalog[value]))
	}
	_, err := tx.Exec(ctx, `INSERT INTO panda_score_weights (model_id, kind, tag_id, value, weight, favorites, catalog)
		SELECT $1, w.kind, NULLIF(w.tag_id, 0), w.value, w.weight, w.favorites, w.catalog
		FROM unnest($2::text[], $3::bigint[], $4::text[], $5::float8[], $6::int[], $7::int[])
			AS w(kind, tag_id, value, weight, favorites, catalog)`,
		candidate.ID, kinds, tagIDs, texts, weightValues, favorites, catalog)
	return candidate, err
}

// storedCalculation reads the stored candidate with the ID, which is not the
// active model, as it was calculated: its weights with their counts and the
// two totals. ok is false when no such candidate is stored.
func storedCalculation(ctx context.Context, tx pgx.Tx, id int64) (calculation scoreCalculation, calculatedAt time.Time, ok bool, err error) {
	err = tx.QueryRow(ctx, "SELECT calculated_at, favorites_in_scope, catalog_in_scope FROM panda_score_models WHERE id = $1 AND NOT active",
		id).Scan(&calculatedAt, &calculation.favoritesInScope, &calculation.catalogInScope)
	if errors.Is(err, pgx.ErrNoRows) {
		return calculation, calculatedAt, false, nil
	}
	if err != nil {
		return calculation, calculatedAt, false, err
	}
	rows, err := tx.Query(ctx, "SELECT kind, COALESCE(tag_id, 0), value, weight, favorites, catalog FROM panda_score_weights WHERE model_id = $1", id)
	if err != nil {
		return calculation, calculatedAt, false, err
	}
	defer rows.Close()
	calculation.weights, calculation.favorites, calculation.catalog = map[scoreValue]float64{}, map[scoreValue]int{}, map[scoreValue]int{}
	for rows.Next() {
		var value scoreValue
		var weight float64
		var favorites, catalog int
		if err := rows.Scan(&value.kind, &value.tagID, &value.value, &weight, &favorites, &catalog); err != nil {
			return calculation, calculatedAt, false, err
		}
		calculation.weights[value], calculation.favorites[value], calculation.catalog[value] = weight, favorites, catalog
	}
	return calculation, calculatedAt, true, rows.Err()
}

// listedWeights returns the strongest weights of the calculation with their
// counts, each with its weight in the active model when that has one.
func listedWeights(ctx context.Context, tx pgx.Tx, calculation scoreCalculation, activeWeights map[scoreValue]float64) ([]collectorapi.ScoreWeight, error) {
	labels, err := tagLabels(ctx, tx, calculation.weights)
	if err != nil {
		return nil, err
	}
	rows := make([]collectorapi.ScoreWeight, 0, len(calculation.weights))
	for value, weight := range calculation.weights {
		label := value.value
		if value.kind == collectorapi.ScoreKindTag {
			label = labels[value.tagID]
		}
		row := collectorapi.ScoreWeight{Kind: value.kind, Label: label, Weight: weight,
			Favorites: calculation.favorites[value], Catalog: calculation.catalog[value]}
		if activeWeight, ok := activeWeights[value]; ok {
			row.ActiveWeight = &activeWeight
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b collectorapi.ScoreWeight) int {
		return cmp.Or(cmp.Compare(b.Weight, a.Weight),
			cmp.Compare(scoreKindOrder[a.Kind], scoreKindOrder[b.Kind]), strings.Compare(a.Label, b.Label))
	})
	return rows[:min(len(rows), scorePreviewWeights)], nil
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
