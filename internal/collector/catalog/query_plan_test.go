package catalog

import (
	"strings"
	"testing"
)

func TestCatalogCategoryQueriesUseIndexes(t *testing.T) {
	db, _ := openCatalog(t)
	for _, tc := range []struct {
		name       string
		categories []string
		visible    bool
	}{
		{"single", []string{"manga"}, true},
		{"multiple", []string{"manga", "doujinshi"}, true},
		{"include expunged", []string{"manga", "doujinshi"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			predicate := "g.category IN (?" + strings.Repeat(",?", len(tc.categories)-1) + ")"
			var args []any
			for _, category := range tc.categories {
				args = append(args, category)
			}
			if tc.visible {
				predicate += " AND g.expunged = 0"
			}
			pageQuery, pageArgs := catalogPageQuery(predicate, args, tc.categories)
			for _, query := range []struct {
				name string
				sql  string
				args []any
			}{
				{"count", "SELECT count(*) FROM catalog_galleries g WHERE " + predicate, args},
				{"page", pageQuery, append(pageArgs, 24, 0)},
			} {
				t.Run(query.name, func(t *testing.T) {
					rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.sql, query.args...)
					if err != nil {
						t.Fatal(err)
					}
					defer rows.Close()
					var covered bool
					for rows.Next() {
						var id, parent, unused int
						var detail string
						if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
							t.Fatal(err)
						}
						t.Log(detail)
						if strings.Contains(detail, "USE TEMP B-TREE") {
							t.Error("category paging must not sort all matching galleries")
						}
						if strings.Contains(detail, "SEARCH g ") || strings.Contains(detail, "SCAN g ") {
							if !strings.Contains(detail, "USING COVERING INDEX") {
								t.Error("category counts and page IDs must not read gallery rows")
							} else {
								covered = true
							}
						}
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					if !covered {
						t.Error("query must use a covering index")
					}
				})
			}
		})
	}
}
