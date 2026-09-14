// Package catalog searches retained Panda metadata independently of collection.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/panda"
)

var ErrInvalidQuery = gallerysearch.ErrInvalidQuery
var ErrInvalidPagination = errors.New("invalid pagination")

type Service struct {
	db            *sql.DB
	galleryOrigin string
}

func New(db *sql.DB, galleryOrigin string) *Service {
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	predicate, args, err := gallerysearch.Predicate(options.Query, namespaces, catalogMatch)
	if err != nil {
		return result, err
	}
	defaultPredicate, defaultArgs, err := gallerysearch.Predicate(options.DefaultQuery, namespaces, catalogMatch)
	if err != nil {
		return result, err
	}
	predicate = "(" + predicate + ") AND (" + defaultPredicate + ")"
	args = append(args, defaultArgs...)
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
		predicate += " AND g.category IN (?" + strings.Repeat(",?", len(categories)-1) + ")"
		for _, category := range categories {
			args = append(args, category)
		}
	}
	if !options.IncludeExpunged {
		predicate += " AND g.expunged = 0"
	}
	if options.Cursor != "" {
		comparison := "<"
		if cursor.Before {
			comparison = ">"
		}
		if cursor.Inclusive {
			comparison += "="
		}
		predicate += " AND (g.posted, g.gallery_id) " + comparison + " (?, ?)"
		args = append(args, cursor.Posted, cursor.GalleryID)
	}
	pageQuery, pageArgs := catalogPageQuery(predicate, args, pageCategories, cursor.Before)
	order := "DESC"
	if cursor.Before {
		order = "ASC"
	}
	rows, err := tx.QueryContext(ctx, `SELECT g.gallery_id, g.title, g.thumbnail_url, g.page_count, g.posted, r.token
		FROM (`+pageQuery+`) page
		JOIN catalog_galleries g ON g.gallery_id = page.gallery_id
		JOIN gallery_refs r ON r.gallery_id = page.gallery_id
		ORDER BY page.posted `+order+`, page.gallery_id `+order,
		append(pageArgs, result.PageSize+1)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item collectorapi.CatalogItem
		var posted int64
		var token string
		if err := rows.Scan(&item.GalleryID, &item.Title, &item.ThumbnailURL, &item.PageCount, &posted, &token); err != nil {
			return result, err
		}
		item.PostedAt = time.Unix(posted, 0).UTC()
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
	return result, tx.Commit()
}

func catalogPageQuery(predicate string, args []any, categories []string, before bool) (string, []any) {
	query := "SELECT g.gallery_id, g.posted FROM catalog_galleries g WHERE " + predicate
	pageArgs := args
	if len(categories) > 1 {
		// Merge ordered category scans so LIMIT can stop them before reading every match.
		branches := make([]string, 0, len(categories))
		pageArgs = make([]any, 0, len(categories)*(len(args)+1)+1)
		for _, category := range categories {
			branches = append(branches, query+" AND g.category = ?")
			pageArgs = append(pageArgs, args...)
			pageArgs = append(pageArgs, category)
		}
		query = strings.Join(branches, " UNION ALL ")
	}
	order := "DESC"
	if before {
		order = "ASC"
	}
	return query + " ORDER BY posted " + order + ", gallery_id " + order + " LIMIT ?", pageArgs
}

func catalogMatch(term gallerysearch.Term) (string, []any) {
	var fields []string
	var args []any
	if term.Field != "tag" {
		fields = append(fields, "instr(g.title_lower, ?) > 0", "instr(g.title_japanese_lower, ?) > 0")
		args = append(args, term.Value, term.Value)
	}
	if term.Field != "title" {
		match, values := gallerysearch.TagPredicate(term, "t.value_lower", "t.namespace")
		if term.Prefix == "-" {
			// Probe the candidate's assignments; even a common excluded tag must not
			// enumerate all of its galleries before the first page can be returned.
			fields = append(fields, `EXISTS (SELECT 1 FROM catalog_gallery_tags gt
				WHERE gt.gallery_id = g.gallery_id AND gt.tag_id IN
				(SELECT t.id FROM catalog_tags t WHERE `+match+"))")
		} else {
			fields = append(fields, `g.gallery_id IN (SELECT gt.gallery_id FROM catalog_tags t
				JOIN catalog_gallery_tags gt ON gt.tag_id = t.id WHERE `+match+")")
		}
		args = append(args, values...)
	}
	return strings.Join(fields, " OR "), args
}

func searchNamespaces(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	names := make(map[string]bool)
	for _, name := range gallerysearch.NamespaceShortForms {
		names[name] = true
	}
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT namespace FROM catalog_tags")
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	result, term, err := gallerysearch.ActiveTerm(query, cursor, namespaces)
	if err != nil || term == nil {
		return result, err
	}
	predicate, args := gallerysearch.TagPredicate(*term, "t.value_lower", "t.namespace")
	rows, err := tx.QueryContext(ctx, `SELECT t.namespace, t.value FROM catalog_tags t WHERE `+predicate+`
		ORDER BY t.value_lower = ? DESC, t.value_lower, t.namespace LIMIT 10`, append(args, term.Value)...)
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
	return result, tx.Commit()
}
