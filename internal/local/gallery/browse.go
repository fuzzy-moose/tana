package gallery

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/fuzzy-moose/tana/internal/panda"
)

type Listing struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int64     `json:"page"`
	PageSize int64     `json:"page_size"`
}

func (r *SQLiteRepository) Browse(ctx context.Context, search string, page, pageSize int64) (Listing, error) {
	return r.BrowseFiltered(ctx, search, page, pageSize, BrowseOptions{})
}

func (r *SQLiteRepository) BrowseFiltered(ctx context.Context, search string, page, pageSize int64, options BrowseOptions) (Listing, error) {
	result := Listing{Items: []Summary{}, Page: page, PageSize: pageSize}
	categories, err := panda.NormalizeCategories(options.Categories)
	if err != nil || !options.Sort.Valid() {
		return result, ErrInvalidQuery
	}
	// Keep counts and the selected page consistent while a scan adds galleries.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	predicate, args, err := searchPredicate(search, namespaces)
	if err != nil {
		return result, err
	}
	options.Categories = categories
	query, err := buildBrowseSQL(predicate, args, options)
	if err != nil {
		return result, err
	}
	err = tx.QueryRowContext(ctx, query.count, query.countArgs...).Scan(&result.Total)
	if err != nil {
		return result, err
	}
	result.Page = min(max(1, page), max(1, (result.Total+pageSize-1)/pageSize))
	rows, err := tx.QueryContext(ctx, query.page,
		append(query.pageArgs, pageSize, (result.Page-1)*pageSize)...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var summary Summary
		if err := rows.Scan(&summary.ID, &summary.Title, &summary.PageCount); err != nil {
			return result, err
		}
		result.Items = append(result.Items, summary)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, tx.Commit()
}

type browseSQL struct {
	count, page         string
	countArgs, pageArgs []any
}

func buildBrowseSQL(predicate string, args []any, options BrowseOptions) (browseSQL, error) {
	facts := options.PandaFacts
	if len(options.Categories) > 0 {
		var sourceIDs []int64
		facts = make([]PandaFact, 0, len(options.PandaFacts))
		for _, fact := range options.PandaFacts {
			if slices.Contains(options.Categories, fact.Category) {
				sourceIDs = append(sourceIDs, fact.SourceID)
				facts = append(facts, fact)
			}
		}
		ids, err := json.Marshal(sourceIDs)
		if err != nil {
			return browseSQL{}, err
		}
		// Category membership is a set of source identities; neither counts nor
		// title ordering need to join the collector's favorite timestamps.
		predicate += " AND g.source_id IN (SELECT value FROM json_each(?))"
		args = append(args, string(ids))
	}
	query := browseSQL{
		count:     "SELECT count(*) FROM galleries g WHERE " + predicate,
		countArgs: args,
	}
	with, from := "", "galleries g"
	order := "title COLLATE NOCASE, id"
	favorite := "NULL"
	if options.Sort == SortFavoritedAsc || options.Sort == SortFavoritedDesc {
		data, err := json.Marshal(facts)
		if err != nil {
			return browseSQL{}, err
		}
		// A JSON relation avoids SQLite's parameter limit and temporary writes.
		// Materialization and integer affinity let SQLite index the source join.
		with = `WITH panda_facts AS MATERIALIZED (
			SELECT CAST(json_extract(value, '$.source_id') AS INTEGER) AS source_id,
				json_extract(value, '$.favorited_at') AS favorited_at
			FROM json_each(?)) `
		args = append([]any{string(data)}, args...)
		from += " LEFT JOIN panda_facts pf ON pf.source_id = g.source_id"
		favorite = "pf.favorited_at"
		switch options.Sort {
		case SortFavoritedAsc:
			order = "favorited_at IS NULL, favorited_at ASC, " + order
		case SortFavoritedDesc:
			order = "favorited_at IS NULL, favorited_at DESC, " + order
		}
	}
	// Count images only after selecting the requested page.
	query.page = with + `SELECT page.id, page.title,
			(SELECT count(*) FROM gallery_pages p WHERE p.gallery_id = page.id)
			FROM (SELECT g.id, g.title, ` + favorite + ` AS favorited_at
				FROM ` + from + ` WHERE ` + predicate + `
				ORDER BY ` + order + ` LIMIT ? OFFSET ?) page ORDER BY ` + order
	query.pageArgs = args
	return query, nil
}
