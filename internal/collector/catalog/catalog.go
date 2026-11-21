// Package catalog searches retained Panda metadata independently of collection.
package catalog

import (
	"cmp"
	"context"
	"errors"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

var ErrInvalidQuery = gallerysearch.ErrInvalidQuery
var ErrInvalidPagination = errors.New("invalid pagination")

type Service struct {
	db              *pgxpool.Pool
	galleryOrigin   string
	scopeBuildBatch int
	scopeWake       chan struct{}
	stopWorkers     context.CancelFunc
	workers         sync.WaitGroup
}

func New(db *pgxpool.Pool, galleryOrigin string) *Service {
	return &Service{db: db, galleryOrigin: strings.TrimRight(galleryOrigin, "/"),
		scopeBuildBatch: defaultScopeBuildBatch, scopeWake: make(chan struct{}, 1)}
}

func (s *Service) List(ctx context.Context, options collectorapi.CatalogOptions) (collectorapi.CatalogResult, error) {
	result := collectorapi.CatalogResult{Items: []collectorapi.CatalogItem{}, PageSize: options.PageSize}
	if options.PageSize < 1 || options.PageSize > 100 {
		return result, ErrInvalidPagination
	}
	cursor, err := decodeCursor(options.Cursor)
	if err != nil {
		return result, err
	}
	if !collectorapi.ValidScoreFilter(options.ScoreFilter) {
		return result, ErrInvalidQuery
	}
	if options.Unseen && cursor.Before {
		// The unseen view has no way back to passed pages.
		return result, ErrInvalidPagination
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	predicate, args, pageCategories, err := catalogPredicate(ctx, tx, options)
	if err != nil {
		return result, err
	}
	if options.Unseen {
		// Probe each candidate in index order; the display table is far too large
		// to read into a list before the first page can be returned.
		predicate += " AND NOT EXISTS (SELECT 1 FROM panda_gallery_displays d WHERE d.gallery_id = g.gallery_id)"
		// Bind the position read beforehand, so the ordered index scan stops there.
		orderedAt, galleryID, ok, err := catchUpPosition(ctx, tx)
		if err != nil {
			return result, err
		}
		if ok {
			predicate += " AND (g.ordered_at, g.gallery_id) > (" + args.Add(orderedAt) + ", " + args.Add(galleryID) + ")"
		}
	}
	filter, err := readScoreFilter(ctx, tx)
	if err != nil {
		return result, err
	}
	scored := filter.inForce()
	if scored {
		result.ScoreFilter = filter.result()
	}
	unseenPredicate, unseenArgs := predicate, slices.Clone(args)
	switch {
	case !scored && options.ScoreFilter == collectorapi.ScoreFilterHidden:
		predicate += " AND false"
	case scored && options.ScoreFilter == "":
		predicate += " AND NOT (" + filter.hidden(&args) + ")"
	case scored && options.ScoreFilter == collectorapi.ScoreFilterHidden:
		predicate += " AND (" + filter.hidden(&args) + ")"
	}
	if options.Cursor != "" {
		comparison := "<"
		if cursor.Before {
			comparison = ">"
		}
		if cursor.Inclusive {
			comparison += "="
		}
		predicate += " AND (g.ordered_at, g.gallery_id) " + comparison + " (" + args.Add(time.Unix(cursor.Posted, 0)) + ", " + args.Add(cursor.GalleryID) + ")"
	}
	pageQuery := catalogPageQuery(predicate, &args, pageCategories, cursor.Before, result.PageSize+1)
	order := "DESC"
	if cursor.Before {
		order = "ASC"
	}
	rows, err := tx.Query(ctx, `SELECT g.gallery_id, g.title, g.thumbnail_url, g.page_count, g.posted, r.token,
		g.metadata_source, g.first_seen_at, r.metadata_error,
		r.feed_metadata_base_at + make_interval(hours => settings.feed_metadata_delay_hours),
		EXISTS (SELECT 1 FROM metadata_fetches f WHERE f.gallery_id = r.gallery_id AND f.token = r.token AND f.status = 'pending')
		FROM (`+pageQuery+`) page
		JOIN catalog_galleries g ON g.gallery_id = page.gallery_id
		JOIN gallery_refs r ON r.gallery_id = page.gallery_id
		JOIN metadata_collection_settings settings ON settings.id = 1
		ORDER BY page.ordered_at `+order+`, page.gallery_id `+order, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item := collectorapi.CatalogItem{SummaryTags: []collectorapi.CatalogTag{}}
		var firstSeen, eligible *time.Time
		var metadataError *string
		var explicitPending bool
		var token string
		if err := rows.Scan(&item.GalleryID, &item.Title, &item.ThumbnailURL, &item.PageCount, &item.PostedAt, &token,
			&item.MetadataSource, &firstSeen, &metadataError, &eligible, &explicitPending); err != nil {
			return result, err
		}
		item.PostedAt = pgdb.UTC(item.PostedAt)
		if item.MetadataSource == "feed" {
			item.MetadataState = "waiting"
			if metadataError != nil && *metadataError != "" && !explicitPending {
				item.MetadataState = "failed"
			}
			item.FirstSeenAt = pgdb.UTC(firstSeen)
			if !explicitPending {
				item.FetchEligibleAt = pgdb.UTC(eligible)
			}
		}
		item.URL = s.GalleryURL(panda.GalleryRef{ID: item.GalleryID, Token: token})
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	more := len(result.Items) > result.PageSize
	if more {
		result.Items = result.Items[:result.PageSize]
	}
	if cursor.Before {
		slices.Reverse(result.Items)
	}
	if err := summarizeTags(ctx, tx, result.Items, filter); err != nil {
		return result, err
	}
	if err := markDisplayed(ctx, tx, result.Items); err != nil {
		return result, err
	}
	if scored {
		if err := filter.markScores(ctx, tx, result.Items); err != nil {
			return result, err
		}
		if options.Unseen && options.ScoreFilter == "" && options.Cursor == "" && len(result.Items) == 0 {
			result.ScoreFilter.HiddenUnseen, err = filter.hasHiddenUnseen(ctx, tx, unseenPredicate, unseenArgs)
			if err != nil {
				return result, err
			}
		}
	}
	hasOpposite := options.Cursor != "" && !cursor.Inclusive
	if options.Unseen {
		// Every non-empty page leads on, even the last: its galleries become seen
		// once the user passes it, and newer galleries wait for the newest page.
		if len(result.Items) > 0 {
			result.NextCursor = itemCursor(result.Items[len(result.Items)-1], false)
		}
	} else if len(result.Items) > 0 {
		if (!cursor.Before && more) || (cursor.Before && hasOpposite) {
			result.NextCursor = itemCursor(result.Items[len(result.Items)-1], false)
		}
		if (cursor.Before && more) || (!cursor.Before && hasOpposite) {
			result.PreviousCursor = itemCursor(result.Items[0], true)
		}
	} else if options.Cursor != "" {
		// Recover from an emptied page without skipping its anchor if it still exists.
		cursor.Before = !cursor.Before
		cursor.Inclusive = true
		if cursor.Before {
			result.PreviousCursor = cursor.encode()
		} else {
			result.NextCursor = cursor.encode()
		}
	}
	return result, tx.Commit(ctx)
}

// catalogPredicate builds the search, default-filter, category and expunged
// conditions on alias g, with their bound arguments and the page categories
// for catalogPageQuery.
func catalogPredicate(ctx context.Context, tx pgx.Tx, options collectorapi.CatalogOptions) (string, pgdb.Args, []string, error) {
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return "", nil, nil, err
	}
	var args pgdb.Args
	matcher := &tagMatcher{ctx: ctx, tx: tx}
	predicate, err := gallerysearch.Predicate(options.Query, namespaces, &args, matcher.match)
	if err != nil {
		return "", nil, nil, err
	}
	defaultPredicate, err := gallerysearch.Predicate(options.DefaultQuery, namespaces, &args, matcher.match)
	if err != nil {
		return "", nil, nil, err
	}
	if matcher.err != nil {
		return "", nil, nil, matcher.err
	}
	predicate = "(" + predicate + ") AND (" + defaultPredicate + ")"
	var pageCategories []string
	for _, selections := range [][]string{options.Categories, options.DefaultCategories} {
		categories, err := panda.NormalizeCategories(selections)
		if err != nil {
			return "", nil, nil, err
		}
		if len(categories) == 0 {
			continue
		}
		if len(pageCategories) == 0 || len(categories) < len(pageCategories) {
			pageCategories = categories
		}
		placeholders := make([]string, 0, len(categories))
		for _, category := range categories {
			placeholders = append(placeholders, args.Add(category))
		}
		predicate += " AND g.category IN (" + strings.Join(placeholders, ", ") + ")"
	}
	if !options.IncludeExpunged {
		predicate += " AND g.expunged IS DISTINCT FROM true"
	}
	return predicate, args, pageCategories, nil
}

// summarizeTags offers each item at most gallerysearch.SummaryTagLimit of its
// tags. While the score filter is not in force, these are its first tags in
// namespace order. While it is in force, all tags of the item are ranked
// first: the tags with a tag score override, "never hide" before "always
// hide", then the tags with an effective weight under the active model by the
// absolute value of their weight, the strongest first, then all other tags;
// ties keep namespace order. Each tag carries its override rule and its weight
// with its source, if it has them, and with an active model whether it is a
// favorite author. Uploader and category weights and overrides play no part.
func summarizeTags(ctx context.Context, tx pgx.Tx, items []collectorapi.CatalogItem, filter scoreFilter) error {
	byID := make(map[int64]*collectorapi.CatalogItem, len(items))
	ids := make([]int64, 0, len(items))
	for i := range items {
		byID[items[i].GalleryID] = &items[i]
		ids = append(ids, items[i].GalleryID)
	}
	args := pgdb.Args{ids}
	weight, source, favorite := "NULL::float8", "''", "false"
	if filter.model != nil {
		// A scalar probe per tag, as in scoreSum, keeps the plan independent of
		// the statistics of a model that was just applied.
		modelID := args.Add(filter.model.id) + "::bigint"
		weight = filter.weightSQL(modelID, collectorapi.ScoreKindTag, "gt.tag_id")
		source = filter.sourceSQL(modelID, collectorapi.ScoreKindTag, "gt.tag_id")
		favorite = favoriteAuthorSQL("gt.tag_id")
	}
	rules := make(map[int64]string)
	for _, override := range filter.overrides {
		if override.Kind == collectorapi.ScoreKindTag {
			rules[override.TagID] = override.Rule
		}
	}
	// Namespace order comes from the database's collation, so the ranking
	// below keeps the row order for ties.
	rows, err := tx.Query(ctx, `SELECT gt.gallery_id, gt.tag_id, t.namespace, t.value, `+weight+`, `+source+`, `+favorite+`
		FROM catalog_gallery_tags gt JOIN catalog_tags t ON t.id = gt.tag_id
		WHERE gt.gallery_id = ANY($1::bigint[]) ORDER BY t.namespace, t.value_lower`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	ranked := filter.inForce()
	for rows.Next() {
		var id, tagID int64
		var tag collectorapi.CatalogTag
		if err := rows.Scan(&id, &tagID, &tag.Namespace, &tag.Value, &tag.Weight, &tag.Source, &tag.FavoriteAuthor); err != nil {
			return err
		}
		tag.Override = rules[tagID]
		if item := byID[id]; ranked || len(item.SummaryTags) < gallerysearch.SummaryTagLimit {
			tag.Term = gallerysearch.SuggestionTerm("", tag.Namespace, tag.Value)
			item.SummaryTags = append(item.SummaryTags, tag)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !ranked {
		return nil
	}
	for _, item := range byID {
		slices.SortStableFunc(item.SummaryTags, func(a, b collectorapi.CatalogTag) int {
			return cmp.Or(cmp.Compare(overrideRank(b), overrideRank(a)), cmp.Compare(tagRank(b), tagRank(a)))
		})
		item.SummaryTags = item.SummaryTags[:min(len(item.SummaryTags), gallerysearch.SummaryTagLimit)]
	}
	return nil
}

// overrideRank puts "never hide" tags before "always hide" tags, and both
// before tags without an override.
func overrideRank(tag collectorapi.CatalogTag) int {
	switch tag.Override {
	case collectorapi.ScoreRuleNeverHide:
		return 2
	case collectorapi.ScoreRuleAlwaysHide:
		return 1
	}
	return 0
}

// tagRank is the absolute weight of a weighted tag without an override, and
// -1 for a tag without a weight, so a weight of 0 still ranks before it.
// Override tags are ranked by overrideRank alone.
func tagRank(tag collectorapi.CatalogTag) float64 {
	if tag.Weight == nil || tag.Override != "" {
		return -1
	}
	return math.Abs(*tag.Weight)
}

func catalogPageQuery(predicate string, args *pgdb.Args, categories []string, before bool, limit int) string {
	order := "DESC"
	if before {
		order = "ASC"
	}
	orderLimit := " ORDER BY ordered_at " + order + ", gallery_id " + order + " LIMIT " + args.Add(limit)
	query := "SELECT g.gallery_id, g.ordered_at FROM catalog_galleries g WHERE " + predicate
	if len(categories) > 1 {
		// Merge ordered category scans that each stop at their own LIMIT; with only
		// the outer LIMIT, the planner reads and sorts every match.
		branches := make([]string, 0, len(categories))
		for _, category := range categories {
			branches = append(branches, "("+query+" AND g.category = "+args.Add(category)+orderLimit+")")
		}
		query = strings.Join(branches, " UNION ALL ")
	}
	return query + orderLimit
}

// tagMatcher resolves tag terms to IDs before the page query is planned. The
// planner cannot probe assignments by primary key for a tag ID it only learns
// from a subquery, so it materializes every gallery carrying that tag instead.
type tagMatcher struct {
	ctx context.Context
	tx  pgx.Tx
	err error
}

func (m *tagMatcher) match(term gallerysearch.Term, args *pgdb.Args) string {
	var fields []string
	if term.Field != "tag" {
		value := args.Add(term.Value)
		fields = append(fields, "strpos(g.title_lower, "+value+") > 0", "strpos(g.title_japanese_lower, "+value+") > 0")
	}
	if term.Field != "title" {
		ids := m.tagIDs(term)
		switch {
		case len(ids) == 0:
			fields = append(fields, "false")
		case term.Prefix == "-":
			// Probe the candidate's assignments; even a common excluded tag must not
			// enumerate all of its galleries before the first page can be returned.
			fields = append(fields, `EXISTS (SELECT 1 FROM catalog_gallery_tags gt
				WHERE gt.gallery_id = g.gallery_id AND gt.tag_id = ANY(`+args.Add(ids)+"))")
		default:
			fields = append(fields, `g.gallery_id IN (SELECT gt.gallery_id FROM catalog_gallery_tags gt
				WHERE gt.tag_id = ANY(`+args.Add(ids)+"))")
		}
	}
	return strings.Join(fields, " OR ")
}

func (m *tagMatcher) tagIDs(term gallerysearch.Term) []int64 {
	if m.err != nil {
		return nil
	}
	var args pgdb.Args
	predicate := gallerysearch.TagPredicate(term, "t.value_lower", "t.namespace", &args)
	ids := []int64{}
	m.err = m.tx.QueryRow(m.ctx, "SELECT COALESCE(array_agg(t.id), '{}') FROM catalog_tags t WHERE "+predicate, args...).Scan(&ids)
	return ids
}

func searchNamespaces(ctx context.Context, tx pgx.Tx) (map[string]bool, error) {
	names := make(map[string]bool)
	for _, name := range gallerysearch.NamespaceShortForms {
		names[name] = true
	}
	// Skip between the few namespaces through the (namespace, value_lower) index;
	// DISTINCT reads the whole tag vocabulary on every search.
	rows, err := tx.Query(ctx, `WITH RECURSIVE ns (namespace) AS (
		SELECT min(namespace) FROM catalog_tags
		UNION ALL
		SELECT (SELECT min(t.namespace) FROM catalog_tags t WHERE t.namespace > ns.namespace) FROM ns WHERE ns.namespace IS NOT NULL
	) SELECT namespace FROM ns WHERE namespace IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names[name] = true
	}
	return names, rows.Err()
}

func (s *Service) Complete(ctx context.Context, query string, cursor int) (collectorapi.CatalogCompletion, error) {
	result := collectorapi.CatalogCompletion{Items: []gallerysearch.TagSuggestion{}}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	result, term, err := gallerysearch.ActiveTerm(query, cursor, namespaces)
	if err != nil || term == nil {
		return result, err
	}
	var args pgdb.Args
	predicate := gallerysearch.TagPredicate(*term, "t.value_lower", "t.namespace", &args)
	exact := args.Add(term.Value)
	rows, err := tx.Query(ctx, `SELECT t.namespace, t.value FROM catalog_tags t WHERE `+predicate+`
		ORDER BY t.value_lower = `+exact+` DESC, t.value_lower, t.namespace LIMIT 10`, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item gallerysearch.TagSuggestion
		if err := rows.Scan(&item.Namespace, &item.Value); err != nil {
			return result, err
		}
		item.Term = gallerysearch.SuggestionTerm(term.Prefix, item.Namespace, item.Value)
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
