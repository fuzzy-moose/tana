package status

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/status/dbgen"
	"github.com/fuzzy-moose/tana/internal/collector/storage"
)

type recordedQuery struct {
	*sql.DB
	query string
}

func (db *recordedQuery) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	db.query = query
	return db.DB.QueryRowContext(ctx, query, args...)
}

func (db *recordedQuery) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	db.query = query
	return db.DB.QueryContext(ctx, query, args...)
}

func TestStatusQueriesAvoidScanningCollectedInventory(t *testing.T) {
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, name := range []string{"inventory", "errors"} {
		t.Run(name, func(t *testing.T) {
			recorded := &recordedQuery{DB: db}
			queries := dbgen.New(recorded)
			if name == "inventory" {
				_, err = queries.InventoryStatistics(t.Context())
			} else {
				_, err = queries.RecentMetadataErrors(t.Context())
			}
			if err != nil {
				t.Fatal(err)
			}
			rows, err := db.Query("EXPLAIN QUERY PLAN " + recorded.query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan []string
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan = append(plan, detail)
				if detail == "SCAN r" || detail == "SCAN gallery_refs" || detail == "SCAN gallery_metadata" {
					t.Errorf("status reads the collected inventory: %s", detail)
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			t.Log(strings.Join(plan, "\n"))
		})
	}
}
