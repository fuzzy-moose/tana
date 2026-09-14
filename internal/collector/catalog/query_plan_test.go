package catalog

import (
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/gallerysearch"
)

func TestCatalogExclusionChecksCandidateTags(t *testing.T) {
	db, _ := openCatalog(t)
	predicate, args, err := gallerysearch.Predicate("-language:translated", map[string]bool{"language": true}, catalogMatch)
	if err != nil {
		t.Fatal(err)
	}
	predicate += " AND g.category IN ('manga', 'doujinshi') AND g.expunged = 0"
	query, args := catalogPageQuery(predicate, args, []string{"manga", "doujinshi"}, false)
	rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, append(args, 25)...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var probes int
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "SEARCH gt ") {
			probes++
			if !strings.Contains(detail, "(gallery_id=? AND tag_id=?)") {
				t.Errorf("exclusion must probe each candidate instead of enumerating excluded galleries: %s", detail)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if probes != 2 {
		t.Fatalf("expected an indexed exclusion probe in each category, got %d", probes)
	}
}

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
			for _, direction := range []string{"first", "next", "previous"} {
				t.Run(direction, func(t *testing.T) {
					predicate := "g.category IN (?" + strings.Repeat(",?", len(tc.categories)-1) + ")"
					var args []any
					for _, category := range tc.categories {
						args = append(args, category)
					}
					if tc.visible {
						predicate += " AND g.expunged = 0"
					}
					if direction != "first" {
						comparison := "<"
						if direction == "previous" {
							comparison = ">"
						}
						predicate += " AND (g.posted, g.gallery_id) " + comparison + " (?, ?)"
						args = append(args, 100, 10)
					}
					query, args := catalogPageQuery(predicate, args, tc.categories, direction == "previous")
					rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query, append(args, 25)...)
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
								t.Error("page IDs must not read gallery rows")
							} else {
								covered = true
							}
							if direction != "first" && !strings.Contains(detail, "AND posted") {
								t.Error("cursor must seek to its upload time")
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
