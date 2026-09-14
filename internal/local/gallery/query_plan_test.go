package gallery

import (
	"strings"
	"testing"
)

func TestBrowseCountsPagesAfterPagination(t *testing.T) {
	db, _, _, _ := openRepositories(t)
	for _, sort := range []BrowseSort{SortTitle, SortFavoritedAsc, SortFavoritedDesc} {
		t.Run(string(sort), func(t *testing.T) {
			query, err := buildBrowseSQL("1", nil, BrowseOptions{Sort: sort})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.page, append(query.pageArgs, 24, 0)...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var selected, counted bool
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				t.Log(detail)
				if strings.HasPrefix(detail, "SCAN page") {
					selected = true
				}
				if strings.HasPrefix(detail, "SEARCH p ") {
					counted = true
					if !selected {
						t.Error("page counts must run only after selecting the requested galleries")
					}
				}
				if sort == SortTitle && !selected && strings.Contains(detail, "USE TEMP B-TREE") {
					t.Error("title pagination must use an index instead of sorting the library")
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !counted {
				t.Fatal("missing page count query")
			}
		})
	}
}

func TestBrowseTagQueriesUseAssignmentIndexes(t *testing.T) {
	db, _, _, _ := openRepositories(t)
	for _, search := range []string{"other:rare$", "tag:rar", "-language:translated"} {
		t.Run(search, func(t *testing.T) {
			predicate, args, err := searchPredicate(search, map[string]bool{"other": true, "language": true})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN SELECT g.id FROM galleries g WHERE "+predicate, args...)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var assignment bool
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				t.Log(detail)
				if strings.HasPrefix(detail, "SEARCH gt ") {
					assignment = true
					want := "(tag_id=?)"
					if strings.HasPrefix(search, "-") {
						want = "(gallery_id=? AND tag_id=?)"
					}
					if !strings.Contains(detail, want) || !strings.Contains(detail, "COVERING INDEX") {
						t.Errorf("tag assignments must use a covering lookup on %s: %s", want, detail)
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !assignment {
				t.Fatal("missing indexed tag lookup")
			}
		})
	}
}

func TestBrowseCountDoesNotJoinFavoriteFacts(t *testing.T) {
	db, _, _, _ := openRepositories(t)
	for _, categories := range [][]string{nil, {"manga", "doujinshi"}} {
		query, err := buildBrowseSQL("1", nil, BrowseOptions{Sort: SortFavoritedDesc, Categories: categories})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := db.QueryContext(t.Context(), "EXPLAIN QUERY PLAN "+query.count, query.countArgs...)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(detail, "panda_facts") || strings.Contains(detail, " pf ") {
				t.Errorf("counts must not load or join favorite times: %s", detail)
			}
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
}
