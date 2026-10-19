package gallery

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/gallerysearch"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

type Listing struct {
	Items    []ListedGallery `json:"items"`
	Total    int64           `json:"total"`
	Page     int64           `json:"page"`
	PageSize int64           `json:"page_size"`
}

// ListedGallery summarizes a gallery for its listing card.
type ListedGallery struct {
	Summary
	SummaryTags []DetailTag `json:"summary_tags"`
}

func (r *Store) Browse(ctx context.Context, search string, page, pageSize int64) (Listing, error) {
	return r.BrowseFiltered(ctx, search, page, pageSize, BrowseOptions{})
}

func (r *Store) BrowseFiltered(ctx context.Context, search string, page, pageSize int64, options BrowseOptions) (Listing, error) {
	result := Listing{Items: []ListedGallery{}, Page: page, PageSize: pageSize}
	categories, err := panda.NormalizeCategories(options.Categories)
	if err != nil || !options.Sort.Valid() {
		return result, ErrInvalidQuery
	}
	// Each statement reads its own committed snapshot, so while a scan adds
	// galleries the total and the selected page may briefly disagree.
	tx, err := r.db.BeginTx(ctx, pgdb.ReadOnly)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	namespaces, err := searchNamespaces(ctx, tx)
	if err != nil {
		return result, err
	}
	var args pgdb.Args
	predicate, err := searchPredicate(search, namespaces, &args)
	if err != nil {
		return result, err
	}
	options.Categories = categories
	query := buildBrowseSQL(predicate, &args, options)
	err = tx.QueryRow(ctx, query.count, args[:query.countArgs]...).Scan(&result.Total)
	if err != nil {
		return result, err
	}
	result.Page = min(max(1, page), max(1, (result.Total+pageSize-1)/pageSize))
	pageQuery := query.page(args.Add(pageSize), args.Add((result.Page-1)*pageSize))
	rows, err := tx.Query(ctx, pageQuery, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		item := ListedGallery{SummaryTags: []DetailTag{}}
		if err := rows.Scan(&item.ID, &item.Title, &item.PageCount); err != nil {
			return result, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if err := summarizeTags(ctx, tx, result.Items); err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}

func summarizeTags(ctx context.Context, tx pgx.Tx, items []ListedGallery) error {
	byID := make(map[int64]*ListedGallery, len(items))
	ids := make([]int64, 0, len(items))
	for i := range items {
		byID[items[i].ID] = &items[i]
		ids = append(ids, items[i].ID)
	}
	rows, err := tx.Query(ctx, `SELECT gt.gallery_id, n.name, t.value
		FROM gallery_tags gt JOIN tags t ON t.id = gt.tag_id
		JOIN namespaces n ON n.id = t.namespace_id
		WHERE gt.gallery_id = ANY($1::bigint[]) ORDER BY n.name, t.value`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var namespace, value string
		if err := rows.Scan(&id, &namespace, &value); err != nil {
			return err
		}
		if item := byID[id]; len(item.SummaryTags) < gallerysearch.SummaryTagLimit {
			item.SummaryTags = append(item.SummaryTags, DetailTag{
				Namespace: namespace, Value: value,
				Term: gallerysearch.SuggestionTerm("", namespace, value),
			})
		}
	}
	return rows.Err()
}

type browseSQL struct {
	count string
	// countArgs is the number of leading arguments the count binds.
	countArgs int
	// page renders the page query for its LIMIT and OFFSET placeholders.
	page func(limit, offset string) string
}

func buildBrowseSQL(predicate string, args *pgdb.Args, options BrowseOptions) browseSQL {
	facts := options.PandaFacts
	if len(options.Categories) > 0 {
		sourceIDs := []int64{}
		facts = make([]PandaFact, 0, len(options.PandaFacts))
		for _, fact := range options.PandaFacts {
			if slices.Contains(options.Categories, fact.Category) {
				sourceIDs = append(sourceIDs, fact.SourceID)
				facts = append(facts, fact)
			}
		}
		// Category membership is a set of source identities; neither counts nor
		// title ordering need to join the collector's favorite timestamps.
		predicate += " AND g.source_id = ANY(" + args.Add(sourceIDs) + "::bigint[])"
	}
	query := browseSQL{
		count:     "SELECT count(*) FROM galleries g WHERE " + predicate,
		countArgs: len(*args),
	}
	from := "galleries g"
	order := "lower(title), id"
	favorite := "NULL::bigint"
	if options.Sort == SortFavoritedAsc || options.Sort == SortFavoritedDesc {
		sourceIDs := make([]int64, 0, len(facts))
		favoritedAt := make([]*int64, 0, len(facts))
		for _, fact := range facts {
			sourceIDs = append(sourceIDs, fact.SourceID)
			favoritedAt = append(favoritedAt, fact.FavoritedAt)
		}
		// Array parameters bind any number of facts without temporary tables.
		from += " LEFT JOIN unnest(" + args.Add(sourceIDs) + "::bigint[], " + args.Add(favoritedAt) +
			"::bigint[]) AS pf(source_id, favorited_at) ON pf.source_id = g.source_id"
		favorite = "pf.favorited_at"
		switch options.Sort {
		case SortFavoritedAsc:
			order = "favorited_at ASC NULLS LAST, " + order
		case SortFavoritedDesc:
			order = "favorited_at DESC NULLS LAST, " + order
		}
	}
	// Count images only after selecting the requested page.
	query.page = func(limit, offset string) string {
		return `SELECT page.id, page.title,
			(SELECT count(*) FROM gallery_pages p WHERE p.gallery_id = page.id)
			FROM (SELECT g.id, g.title, ` + favorite + ` AS favorited_at
				FROM ` + from + ` WHERE ` + predicate + `
				ORDER BY ` + order + ` LIMIT ` + limit + ` OFFSET ` + offset + `) page ORDER BY ` + order
	}
	return query
}
