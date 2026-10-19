// Package catalog searches retained Panda metadata independently of collection.
package catalog

import (
	"context"
	"errors"
	"slices"
	"strings"
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
	db            *pgxpool.Pool
	galleryOrigin string
}

func New(db *pgxpool.Pool, galleryOrigin string) *Service {
	return &Service{db: db, galleryOrigin: strings.TrimRight(galleryOrigin, "/")}
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
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	var args pgdb.Args
	matcher := &tagMatcher{ctx: ctx, tx: tx}
	predicate, err := gallerysearch.Predicate(options.Query, namespaces, &args, matcher.match)
	if err != nil {
		return result, err
	}
	defaultPredicate, err := gallerysearch.Predicate(options.DefaultQuery, namespaces, &args, matcher.match)
	if err != nil {
		return result, err
	}
	if matcher.err != nil {
		return result, matcher.err
	}
	predicate = "(" + predicate + ") AND (" + defaultPredicate + ")"
	var pageCategories []string
	for _, selections := range [][]string{options.Categories, options.DefaultCategories} {
		categories, err := panda.NormalizeCategories(selections)
		if err != nil {
			return result, err
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
		var item collectorapi.CatalogItem
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
	hasOpposite := options.Cursor != "" && !cursor.Inclusive
	if len(result.Items) > 0 {
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
