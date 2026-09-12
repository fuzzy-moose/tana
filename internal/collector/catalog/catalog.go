// Package catalog searches retained Panda metadata independently of collection.
package catalog

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/gallerysearch"
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
	result := collectorapi.CatalogResult{Items: []collectorapi.CatalogItem{}, Page: options.Page, PageSize: options.PageSize}
	if options.Page < 1 || options.PageSize < 1 || options.PageSize > 100 {
		return result, ErrInvalidPagination
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
	if !options.IncludeExpunged {
		predicate += " AND g.expunged = 0"
	}
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM catalog_galleries g WHERE "+predicate, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	result.TotalPages = max(1, int((result.Total+int64(options.PageSize)-1)/int64(options.PageSize)))
	result.Page = min(options.Page, result.TotalPages)
	rows, err := tx.QueryContext(ctx, `SELECT g.gallery_id, g.title, g.thumbnail_url, g.page_count, g.posted, r.token
		FROM catalog_galleries g JOIN gallery_refs r ON r.gallery_id = g.gallery_id
		WHERE `+predicate+` ORDER BY g.posted DESC, g.gallery_id DESC LIMIT ? OFFSET ?`,
		append(args, result.PageSize, int64(result.Page-1)*int64(result.PageSize))...)
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
		item.URL = s.galleryOrigin + "/g/" + strconv.FormatInt(item.GalleryID, 10) + "/" + url.PathEscape(token) + "/"
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit()
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
		fields = append(fields, `g.gallery_id IN (SELECT gt.gallery_id FROM catalog_tags t
			JOIN catalog_gallery_tags gt ON gt.tag_id = t.id WHERE `+match+")")
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
