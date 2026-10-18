package enrichment

import (
	"context"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestEnrichmentHeldByAnotherTransactionIsSkipped(t *testing.T) {
	s := openService(t, nil, func(context.Context, []int64) (collectorapi.LookupResult, error) {
		return collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{
			{Metadata: panda.Metadata{ID: 1, Title: "First"}},
			{Metadata: panda.Metadata{ID: 2, Title: "Second"}},
		}}, nil
	})
	first, second := importGallery(t, s, 1), importGallery(t, s, 2)
	held := pgtest.Begin(t, s.db)
	if _, err := held.Exec(t.Context(), "SELECT 1 FROM panda_enrichments WHERE gallery_id = $1 FOR UPDATE", first); err != nil {
		t.Fatal(err)
	}
	step(t, s, time.Now())
	pendingCount(t, s.db, 1)
	checkGallery(t, s, first, "Snapshot", "other:snapshot")
	checkGallery(t, s, second, "Second")
	if err := held.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	step(t, s, time.Now())
	pendingCount(t, s.db, 0)
	checkGallery(t, s, first, "First")
}
