package catalog

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb"
)

// The catalog in scope of the default filter is a projection: a build counts
// the catalog once, in batches of scopeBuildBatch galleries, and every later
// catalog change is counted from catalog_scope_changes. A score model
// calculation reads the counts instead of counting the catalog, which takes
// more than a minute on the real catalog.
const (
	// defaultScopeBuildBatch is the number of galleries a build counts in one
	// transaction; a preview waits for at most one batch.
	defaultScopeBuildBatch = 10000
	// scopeChangeBatch is the number of changed galleries counted in one
	// transaction.
	scopeChangeBatch = 1000
	// scopeIdleDelay is how long the worker waits for changes once it is
	// idle.
	scopeIdleDelay = 5 * time.Second
	// scopeRetryDelay is how long the worker waits after a failed step.
	scopeRetryDelay = 30 * time.Second
)

// catalogScope is the catalog_scope row. query is nil until a preview names
// a default filter.
type catalogScope struct {
	query        *string
	categories   []string
	builtThrough int64
	ready        bool
	galleries    int
}

func readScope(ctx context.Context, tx pgx.Tx, lock string) (catalogScope, error) {
	var scope catalogScope
	err := tx.QueryRow(ctx, "SELECT query, categories, built_through, ready, galleries FROM catalog_scope WHERE id = 1 "+lock).
		Scan(&scope.query, &scope.categories, &scope.builtThrough, &scope.ready, &scope.galleries)
	return scope, err
}

// is reports whether the scope is that of the default filter, with the
// categories as scopeCategories gives them.
func (s catalogScope) is(query string, categories []string) bool {
	return s.query != nil && *s.query == query && slices.Equal(s.categories, categories)
}

func (s catalogScope) options() collectorapi.CatalogOptions {
	return collectorapi.CatalogOptions{DefaultQuery: *s.query, DefaultCategories: s.categories, IncludeExpunged: true}
}

// scopeCategories normalizes and sorts the categories of a default filter, so
// that the same selection always names the same scope.
func scopeCategories(categories []string) ([]string, error) {
	normalized, err := panda.NormalizeCategories(categories)
	if err != nil {
		return nil, err
	}
	slices.Sort(normalized)
	return normalized, nil
}

// scopeGallery selects, for the galleries that pass the WHERE clause that
// follows it, the values a gallery is counted with.
const scopeGallery = `SELECT g.gallery_id, g.uploader, g.category,
		ARRAY(SELECT gt.tag_id FROM catalog_gallery_tags gt WHERE gt.gallery_id = g.gallery_id ORDER BY gt.tag_id)
	FROM catalog_galleries g WHERE `

// useScope makes the default filter the scope of the counts and returns its
// categories as scopeCategories gives them. A filter other than the current
// one drops the counts and starts a new build; an invalid one changes
// nothing. Waiting changes stay: once the new build ends, each is counted
// again from what the build counted.
func (s *Service) useScope(ctx context.Context, query string, categories []string) ([]string, error) {
	categories, err := scopeCategories(categories)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	scope, err := readScope(ctx, tx, "FOR UPDATE")
	if err != nil || scope.is(query, categories) {
		return categories, err
	}
	if _, _, _, err := catalogPredicate(ctx, tx, collectorapi.CatalogOptions{DefaultQuery: query, DefaultCategories: categories}); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, "TRUNCATE catalog_scope_galleries, catalog_scope_counts"); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE catalog_scope SET query = $1, categories = $2, built_through = 0, ready = false, galleries = 0
		WHERE id = 1`, query, categories); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	select {
	case s.scopeWake <- struct{}{}:
	default:
	}
	return categories, nil
}

// scopeProgress is the share of the catalog, in percent and by gallery ID,
// that the build has counted.
func scopeProgress(ctx context.Context, tx pgx.Tx, scope catalogScope) (int, error) {
	var last *int64
	if err := tx.QueryRow(ctx, "SELECT max(gallery_id) FROM catalog_galleries").Scan(&last); err != nil {
		return 0, err
	}
	if last == nil || *last <= 0 {
		return 0, nil
	}
	return int(min(99, scope.builtThrough*100 / *last)), nil
}

// UpdateScopeCounts does one step of the scope count work: a build batch
// while the counts are not ready, else one batch of changed galleries. It
// reports whether work is left.
func (s *Service) UpdateScopeCounts(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	scope, err := readScope(ctx, tx, "FOR UPDATE")
	if err != nil {
		return false, err
	}
	var more bool
	if scope.query == nil {
		// Nothing is counted, so a change needs no counting.
		tag, err := tx.Exec(ctx, `DELETE FROM catalog_scope_changes WHERE gallery_id IN (
			SELECT gallery_id FROM catalog_scope_changes ORDER BY gallery_id LIMIT $1 FOR UPDATE SKIP LOCKED)`, scopeChangeBatch)
		if err != nil {
			return false, err
		}
		more = tag.RowsAffected() == scopeChangeBatch
	} else {
		predicate, args, _, err := catalogPredicate(ctx, tx, scope.options())
		if err != nil {
			return false, err
		}
		if !scope.ready {
			// The changes during the build wait until it ends: each changed
			// gallery is then counted again from what the build counted.
			err = s.buildScope(ctx, tx, scope, predicate, args)
			more = true
		} else {
			more, err = countScopeChanges(ctx, tx, predicate, args)
		}
		if err != nil {
			return false, err
		}
	}
	return more, tx.Commit(ctx)
}

// buildScope counts the next batch of catalog galleries by ID, or marks the
// counts ready when none is left.
func (s *Service) buildScope(ctx context.Context, tx pgx.Tx, scope catalogScope, predicate string, args pgdb.Args) error {
	var through *int64
	if err := tx.QueryRow(ctx, `SELECT max(gallery_id) FROM (
		SELECT gallery_id FROM catalog_galleries WHERE gallery_id > $1 ORDER BY gallery_id LIMIT $2) batch`,
		scope.builtThrough, s.scopeBuildBatch).Scan(&through); err != nil {
		return err
	}
	if through == nil {
		_, err := tx.Exec(ctx, "UPDATE catalog_scope SET ready = true WHERE id = 1")
		return err
	}
	args = slices.Clone(args)
	after, last := args.Add(scope.builtThrough), args.Add(*through)
	_, err := tx.Exec(ctx, `WITH added AS (
			INSERT INTO catalog_scope_galleries (gallery_id, uploader, category, tag_ids)
			`+scopeGallery+`g.gallery_id > `+after+` AND g.gallery_id <= `+last+` AND (`+predicate+`)
			RETURNING uploader, category, tag_ids
		), carried AS (
			SELECT 'tag' AS kind, unnest(tag_ids) AS tag_id, '' AS value FROM added
			UNION ALL SELECT 'uploader', 0, uploader FROM added WHERE uploader <> ''
			UNION ALL SELECT 'category', 0, category FROM added WHERE category <> ''
		), counted AS (
			INSERT INTO catalog_scope_counts (kind, tag_id, value, galleries)
			SELECT kind, tag_id, value, count(*) FROM carried GROUP BY kind, tag_id, value
			ON CONFLICT (kind, tag_id, value) DO UPDATE SET galleries = catalog_scope_counts.galleries + excluded.galleries
		)
		UPDATE catalog_scope SET built_through = `+last+`, galleries = galleries + (SELECT count(*) FROM added) WHERE id = 1`, args...)
	return err
}

// countScopeChanges counts one batch of changed galleries again: it takes
// back what each counted and counts it as the catalog now has it. A gallery
// whose change is not committed yet holds its change row and is skipped; its
// change is counted in a later batch. It reports whether changes are left.
func countScopeChanges(ctx context.Context, tx pgx.Tx, predicate string, args pgdb.Args) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT gallery_id FROM catalog_scope_changes
		ORDER BY gallery_id LIMIT $1 FOR UPDATE SKIP LOCKED`, scopeChangeBatch)
	if err != nil {
		return false, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil || len(ids) == 0 {
		return false, err
	}
	delta := map[scoreValue]int{}
	galleries := 0
	count := func(query string, args pgdb.Args, sign int) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		// A counted gallery carries its values as a favorite does.
		counted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (favoriteGallery, error) {
			var gallery favoriteGallery
			err := row.Scan(&gallery.uploader, &gallery.category, &gallery.tagIDs)
			return gallery, err
		})
		if err != nil {
			return err
		}
		for _, gallery := range counted {
			galleries += sign
			for _, value := range gallery.values() {
				delta[value] += sign
			}
		}
		return nil
	}
	if err := count(`DELETE FROM catalog_scope_galleries WHERE gallery_id = ANY($1::bigint[])
		RETURNING uploader, category, tag_ids`, pgdb.Args{ids}, -1); err != nil {
		return false, err
	}
	args = slices.Clone(args)
	idsArg := args.Add(ids)
	if err := count(`INSERT INTO catalog_scope_galleries (gallery_id, uploader, category, tag_ids)
		`+scopeGallery+`g.gallery_id = ANY(`+idsArg+`::bigint[]) AND (`+predicate+`)
		RETURNING uploader, category, tag_ids`, args, 1); err != nil {
		return false, err
	}
	var kinds, texts []string
	var tagIDs []int64
	var changes []int32
	for value, change := range delta {
		if change != 0 {
			kinds, tagIDs, texts, changes = append(kinds, value.kind), append(tagIDs, value.tagID), append(texts, value.value), append(changes, int32(change))
		}
	}
	if len(kinds) > 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO catalog_scope_counts (kind, tag_id, value, galleries)
			SELECT * FROM unnest($1::text[], $2::bigint[], $3::text[], $4::int[])
			ON CONFLICT (kind, tag_id, value) DO UPDATE SET galleries = catalog_scope_counts.galleries + excluded.galleries`,
			kinds, tagIDs, texts, changes); err != nil {
			return false, err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM catalog_scope_counts c
			USING unnest($1::text[], $2::bigint[], $3::text[]) AS k(kind, tag_id, value)
			WHERE c.kind = k.kind AND c.tag_id = k.tag_id AND c.value = k.value AND c.galleries = 0`,
			kinds, tagIDs, texts); err != nil {
			return false, err
		}
	}
	if _, err := tx.Exec(ctx, "UPDATE catalog_scope SET galleries = galleries + $1 WHERE id = 1", galleries); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM catalog_scope_changes WHERE gallery_id = ANY($1::bigint[])", ids); err != nil {
		return false, err
	}
	return len(ids) == scopeChangeBatch, nil
}

// storedScopeCounts reads the stored count of each of the values in the
// catalog in scope.
func storedScopeCounts(ctx context.Context, tx pgx.Tx, values []scoreValue) (map[scoreValue]int, error) {
	var tagIDs []int64
	var uploaders, categories []string
	for _, value := range values {
		switch value.kind {
		case collectorapi.ScoreKindTag:
			tagIDs = append(tagIDs, value.tagID)
		case collectorapi.ScoreKindUploader:
			uploaders = append(uploaders, value.value)
		case collectorapi.ScoreKindCategory:
			categories = append(categories, value.value)
		}
	}
	rows, err := tx.Query(ctx, `SELECT kind, tag_id, value, galleries FROM catalog_scope_counts
		WHERE kind = 'tag' AND tag_id = ANY($1::bigint[]) AND value = ''
		UNION ALL SELECT kind, tag_id, value, galleries FROM catalog_scope_counts
		WHERE kind = 'uploader' AND tag_id = 0 AND value = ANY($2::text[])
		UNION ALL SELECT kind, tag_id, value, galleries FROM catalog_scope_counts
		WHERE kind = 'category' AND tag_id = 0 AND value = ANY($3::text[])`, tagIDs, uploaders, categories)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[scoreValue]int, len(values))
	for rows.Next() {
		var value scoreValue
		var count int
		if err := rows.Scan(&value.kind, &value.tagID, &value.value, &count); err != nil {
			return nil, err
		}
		counts[value] = count
	}
	return counts, rows.Err()
}

// StartScopeCounts starts the worker that keeps the scope counts. Close stops
// it.
func (s *Service) StartScopeCounts(ctx context.Context, logger *slog.Logger) {
	ctx, cancel := context.WithCancel(ctx)
	s.stopWorkers = cancel
	s.workers.Go(func() {
		for ctx.Err() == nil {
			more, err := s.UpdateScopeCounts(ctx)
			delay := scopeIdleDelay
			switch {
			case err != nil && ctx.Err() == nil:
				logger.Error("catalog_scope_counts_failed", "error", err)
				delay = scopeRetryDelay
			case more:
				continue
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
			case <-s.scopeWake:
			case <-timer.C:
			}
			timer.Stop()
		}
	})
}

// Close stops the background work of the service.
func (s *Service) Close() {
	if s.stopWorkers != nil {
		s.stopWorkers()
	}
	s.workers.Wait()
}
