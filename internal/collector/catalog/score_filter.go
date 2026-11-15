package catalog

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// scoreItemWeights is the number of the strongest weights an item reports.
const scoreItemWeights = 5

// scoreFilter is the state of the score filter read inside one List
// transaction. It is in force when a model is active or at least one override
// exists; pins and the author bonus alone never put it in force.
type scoreFilter struct {
	overrides   []scoreOverride
	pins        []scorePin
	authorBonus *float64
	model       *activeScoreModel
}

type activeScoreModel struct {
	id              int64
	thresholdTenths int
	favorites       bool
	calculatedAt    time.Time
}

func readScoreFilter(ctx context.Context, tx pgx.Tx) (scoreFilter, error) {
	overrides, err := scoreOverrides(ctx, tx)
	if err != nil {
		return scoreFilter{}, err
	}
	// Kind order tag, uploader, category, then label decides which override
	// an item reports.
	slices.SortFunc(overrides, func(a, b scoreOverride) int {
		return cmp.Or(cmp.Compare(scoreKindOrder[a.Kind], scoreKindOrder[b.Kind]), strings.Compare(a.Label, b.Label))
	})
	pins, err := scorePins(ctx, tx)
	if err != nil {
		return scoreFilter{}, err
	}
	bonus, err := authorBonus(ctx, tx)
	if err != nil {
		return scoreFilter{}, err
	}
	filter := scoreFilter{overrides: overrides, pins: pins, authorBonus: bonus}
	var model activeScoreModel
	err = tx.QueryRow(ctx, `SELECT id, threshold_tenths, favorites, calculated_at
		FROM panda_score_models WHERE active`).Scan(&model.id, &model.thresholdTenths, &model.favorites, &model.calculatedAt)
	switch {
	case err == nil:
		filter.model = &model
	case !errors.Is(err, pgx.ErrNoRows):
		return scoreFilter{}, err
	}
	return filter, nil
}

var scoreKindOrder = map[string]int{collectorapi.ScoreKindTag: 0, collectorapi.ScoreKindUploader: 1, collectorapi.ScoreKindCategory: 2}

func (f scoreFilter) inForce() bool {
	return len(f.overrides) > 0 || f.model != nil
}

// result describes the filter for a catalog page.
func (f scoreFilter) result() *collectorapi.CatalogScoreFilter {
	result := &collectorapi.CatalogScoreFilter{}
	if f.model != nil {
		threshold := f.model.thresholdTenths
		result.ThresholdTenths = &threshold
	}
	return result
}

// scoreTenths returns floor(score * 10) of the gallery on alias g under the
// model whose ID the placeholder binds. The score is the sum of the effective
// weights of all the gallery's tags, its uploader and its category; it probes
// each of them by key. Weights and pins never have an empty value, so an empty
// uploader or category adds nothing. Every comparison with a threshold and
// every histogram uses this one expression, so they agree at a bucket border.
func (f scoreFilter) scoreTenths(modelID string) string {
	return "floor(" + f.scoreSum(modelID) + " * 10)"
}

// scoreSum probes the weight of each of the gallery's tags in a scalar
// subquery. With a join instead, a candidate stored in the same transaction
// has no statistics, and the planner scans all its tag weights per gallery.
func (f scoreFilter) scoreSum(modelID string) string {
	return `(COALESCE((SELECT sum(` + f.weightSQL(modelID, collectorapi.ScoreKindTag, "gt.tag_id") + `)
			FROM catalog_gallery_tags gt WHERE gt.gallery_id = g.gallery_id), 0)
		+ COALESCE(` + f.weightSQL(modelID, collectorapi.ScoreKindUploader, "g.uploader") + `, 0)
		+ COALESCE(` + f.weightSQL(modelID, collectorapi.ScoreKindCategory, "g.category") + `, 0))`
}

// weightSQL returns the SQL expression for the effective weight of a value of
// the kind under the model whose ID the placeholder binds, NULL for a value
// without one: its pin, else for the tag of a favorite author while the author
// bonus is on the larger of its weight in the model and the bonus, else its
// weight in the model. The key is an expression for the catalog_tags ID of a
// tag, or for the value of an uploader or a category. Every reader of a weight
// takes it from here, so a card's weights agree with the filter. While no pin
// exists, the pin probe is left out, and while the bonus is off, the favorite
// author probe.
func (f scoreFilter) weightSQL(modelID, kind, key string) string {
	weight := calculatedSQL(modelID, kind, key)
	if bonus, ok := f.bonusSQL(kind); ok {
		// GREATEST ignores NULL, so a tag without a calculated weight gets the
		// bonus.
		weight = "CASE WHEN " + favoriteAuthorSQL(key) + " THEN GREATEST(" + weight + ", " + bonus + ") ELSE " + weight + " END"
	}
	if len(f.pins) == 0 {
		return weight
	}
	return "COALESCE(" + pinSQL(kind, key) + ", " + weight + ")"
}

// sourceSQL returns the SQL expression for the source of the effective weight
// that weightSQL gives for the same value: ScoreSourcePin, else
// ScoreSourceAuthorBonus where the bonus is above the weight in the model or
// the model has none, else empty.
func (f scoreFilter) sourceSQL(modelID, kind, key string) string {
	var cases string
	if len(f.pins) > 0 {
		cases += " WHEN " + pinSQL(kind, key) + " IS NOT NULL THEN '" + collectorapi.ScoreSourcePin + "'"
	}
	if bonus, ok := f.bonusSQL(kind); ok {
		cases += " WHEN " + favoriteAuthorSQL(key) + " AND COALESCE(" + calculatedSQL(modelID, kind, key) + " < " + bonus + ", true)" +
			" THEN '" + collectorapi.ScoreSourceAuthorBonus + "'"
	}
	if cases == "" {
		return "''"
	}
	return "CASE" + cases + " ELSE '' END"
}

// calculatedSQL probes the weight of a value in the model whose ID the
// placeholder binds, NULL without one.
func calculatedSQL(modelID, kind, key string) string {
	column := "w.value"
	if kind == collectorapi.ScoreKindTag {
		column = "w.tag_id"
	}
	return `(SELECT w.weight FROM panda_score_weights w
		WHERE w.model_id = ` + modelID + ` AND w.kind = '` + kind + `' AND ` + column + ` = ` + key + `)`
}

// bonusSQL returns the author bonus as an SQL literal, when it is on and
// applies to the kind. The bonus is a finite float, so its literal is exact.
func (f scoreFilter) bonusSQL(kind string) (string, bool) {
	if f.authorBonus == nil || kind != collectorapi.ScoreKindTag {
		return "", false
	}
	return "'" + strconv.FormatFloat(*f.authorBonus, 'g', -1, 64) + "'::float8", true
}

// favoriteAuthorSQL is the condition that the tag with the catalog_tags ID is
// a favorite author, probed by the primary key of favorite_authors.
func favoriteAuthorSQL(key string) string {
	return "EXISTS (SELECT 1 FROM favorite_authors fa WHERE fa.tag_id = " + key + ")"
}

// pinSQL probes the pin of a value through the unique indexes of
// panda_score_pins, NULL without one.
func pinSQL(kind, key string) string {
	if kind == collectorapi.ScoreKindTag {
		return "(SELECT p.weight FROM panda_score_pins p WHERE p.kind = 'tag' AND p.tag_id = " + key + ")"
	}
	return "(SELECT p.weight FROM panda_score_pins p WHERE p.kind = '" + kind + "' AND p.value = " + key + ")"
}

// belowThreshold returns the condition on alias g that the active model's
// score is below its threshold.
func (f scoreFilter) belowThreshold(args *pgdb.Args) string {
	if f.model == nil {
		return "false"
	}
	return f.scoreTenths(args.Add(f.model.id)+"::bigint") + " < " + args.Add(f.model.thresholdTenths) + "::integer"
}

// hidden returns the condition on alias g that the filter hides the gallery:
// its verdict decides first, then a "never hide" match, then an "always hide"
// match, then a score below the threshold. Each part probes the candidate by
// key, so the ordered catalog scan stops at its limit.
func (f scoreFilter) hidden(args *pgdb.Args) string {
	never := f.matches(collectorapi.ScoreRuleNeverHide, args)
	always := f.matches(collectorapi.ScoreRuleAlwaysHide, args)
	below := f.belowThreshold(args)
	return `COALESCE((SELECT v.verdict = ` + args.Add(collectorapi.VerdictNotForMe) + `
		FROM panda_gallery_verdicts v WHERE v.gallery_id = g.gallery_id), NOT (` + never + `) AND (` + always + ` OR ` + below + `))`
}

// matches returns the condition that an override with the rule matches one of
// the gallery's tags, its uploader or its category. Override values are never
// empty, so an empty uploader or category matches none.
func (f scoreFilter) matches(rule string, args *pgdb.Args) string {
	var tagIDs []int64
	var uploaders, categories []string
	for _, override := range f.overrides {
		if override.Rule != rule {
			continue
		}
		switch override.Kind {
		case collectorapi.ScoreKindTag:
			tagIDs = append(tagIDs, override.TagID)
		case collectorapi.ScoreKindUploader:
			uploaders = append(uploaders, override.Label)
		case collectorapi.ScoreKindCategory:
			categories = append(categories, override.Label)
		}
	}
	conditions := []string{}
	if len(tagIDs) > 0 {
		conditions = append(conditions, `EXISTS (SELECT 1 FROM catalog_gallery_tags gt
			WHERE gt.gallery_id = g.gallery_id AND gt.tag_id = ANY(`+args.Add(tagIDs)+`::bigint[]))`)
	}
	if len(uploaders) > 0 {
		conditions = append(conditions, "g.uploader = ANY("+args.Add(uploaders)+"::text[])")
	}
	if len(categories) > 0 {
		conditions = append(conditions, "g.category = ANY("+args.Add(categories)+"::text[])")
	}
	if len(conditions) == 0 {
		return "false"
	}
	return "(" + strings.Join(conditions, " OR ") + ")"
}

// markScores sets Score on the page's items, never on every candidate: the
// first of the "never hide", "always hide" and threshold rules that matches,
// without regard to the verdict, with the label of the deciding override, and
// with an active model the item's score and its strongest weights.
func (f scoreFilter) markScores(ctx context.Context, tx pgx.Tx, items []collectorapi.CatalogItem) error {
	byID := make(map[int64]*collectorapi.CatalogItem, len(items))
	ids := make([]int64, 0, len(items))
	for i := range items {
		byID[items[i].GalleryID] = &items[i]
		ids = append(ids, items[i].GalleryID)
	}
	args := pgdb.Args{ids}
	value, below := "NULL::float8", "false"
	if f.model != nil {
		value = f.scoreSum(args.Add(f.model.id) + "::bigint")
		below = f.belowThreshold(&args)
	}
	rows, err := tx.Query(ctx, `SELECT g.gallery_id, g.uploader, g.category,
			ARRAY(SELECT gt.tag_id FROM catalog_gallery_tags gt WHERE gt.gallery_id = g.gallery_id),
			`+value+`, `+below+`
		FROM catalog_galleries g WHERE g.gallery_id = ANY($1::bigint[])`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var uploader, category string
		var tagIDs []int64
		var score *float64
		var isBelow bool
		if err := rows.Scan(&id, &uploader, &category, &tagIDs, &score, &isBelow); err != nil {
			return err
		}
		itemScore := &collectorapi.CatalogItemScore{Value: score}
		if override, ok := f.decidingOverride(uploader, category, tagIDs); ok {
			itemScore.Rule, itemScore.Override = override.Rule, override.Label
		} else if isBelow {
			itemScore.Rule = collectorapi.ScoreRuleBelowThreshold
		}
		if itemScore.Rule != "" || itemScore.Value != nil {
			byID[id].Score = itemScore
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if f.model == nil {
		return nil
	}
	return f.markWeights(ctx, tx, byID, ids)
}

// markWeights gives each scored item its strongest effective weights among
// all its tags, its uploader and its category, each with its source.
func (f scoreFilter) markWeights(ctx context.Context, tx pgx.Tx, byID map[int64]*collectorapi.CatalogItem, ids []int64) error {
	modelID := "$2::bigint"
	rows, err := tx.Query(ctx, `SELECT gallery_id, kind, label, weight, source FROM (
			SELECT gt.gallery_id, '`+collectorapi.ScoreKindTag+`' AS kind, t.namespace || ':' || t.value AS label,
				`+f.weightSQL(modelID, collectorapi.ScoreKindTag, "gt.tag_id")+` AS weight,
				`+f.sourceSQL(modelID, collectorapi.ScoreKindTag, "gt.tag_id")+` AS source
			FROM catalog_gallery_tags gt JOIN catalog_tags t ON t.id = gt.tag_id
			WHERE gt.gallery_id = ANY($1::bigint[])
			UNION ALL
			SELECT g.gallery_id, '`+collectorapi.ScoreKindUploader+`', g.uploader,
				`+f.weightSQL(modelID, collectorapi.ScoreKindUploader, "g.uploader")+`,
				`+f.sourceSQL(modelID, collectorapi.ScoreKindUploader, "g.uploader")+`
			FROM catalog_galleries g WHERE g.gallery_id = ANY($1::bigint[])
			UNION ALL
			SELECT g.gallery_id, '`+collectorapi.ScoreKindCategory+`', g.category,
				`+f.weightSQL(modelID, collectorapi.ScoreKindCategory, "g.category")+`,
				`+f.sourceSQL(modelID, collectorapi.ScoreKindCategory, "g.category")+`
			FROM catalog_galleries g WHERE g.gallery_id = ANY($1::bigint[])
		) weighted WHERE weight IS NOT NULL`, ids, f.model.id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var weight collectorapi.CatalogScoreWeight
		if err := rows.Scan(&id, &weight.Kind, &weight.Label, &weight.Weight, &weight.Source); err != nil {
			return err
		}
		if score := byID[id].Score; score != nil {
			score.Weights = append(score.Weights, weight)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range byID {
		if item.Score == nil {
			continue
		}
		slices.SortFunc(item.Score.Weights, func(a, b collectorapi.CatalogScoreWeight) int {
			return cmp.Or(cmp.Compare(math.Abs(b.Weight), math.Abs(a.Weight)),
				cmp.Compare(scoreKindOrder[a.Kind], scoreKindOrder[b.Kind]), strings.Compare(a.Label, b.Label))
		})
		item.Score.Weights = item.Score.Weights[:min(len(item.Score.Weights), scoreItemWeights)]
	}
	return nil
}

func (f scoreFilter) decidingOverride(uploader, category string, tagIDs []int64) (scoreOverride, bool) {
	for _, rule := range []string{collectorapi.ScoreRuleNeverHide, collectorapi.ScoreRuleAlwaysHide} {
		for _, override := range f.overrides {
			if override.Rule != rule {
				continue
			}
			var match bool
			switch override.Kind {
			case collectorapi.ScoreKindTag:
				match = slices.Contains(tagIDs, override.TagID)
			case collectorapi.ScoreKindUploader:
				match = uploader != "" && override.Label == uploader
			case collectorapi.ScoreKindCategory:
				match = category != "" && override.Label == category
			}
			if match {
				return override, true
			}
		}
	}
	return scoreOverride{}, false
}

// hasHiddenUnseen reports whether at least one gallery that passes the
// predicate, which holds the unseen conditions, is hidden.
func (f scoreFilter) hasHiddenUnseen(ctx context.Context, tx pgx.Tx, predicate string, args pgdb.Args) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM catalog_galleries g WHERE "+predicate+" AND ("+f.hidden(&args)+"))", args...).Scan(&found)
	return found, err
}
