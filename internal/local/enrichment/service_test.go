package enrichment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/enrichment/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/local/tag"
	"github.com/fuzzy-moose/tana/internal/panda"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

type lookupFunc func(context.Context, []int64) (collectorapi.LookupResult, error)

func (f lookupFunc) Lookup(ctx context.Context, ids []int64) (collectorapi.LookupResult, error) {
	return f(ctx, ids)
}

// openService uses db, or a new database when db is nil.
func openService(t *testing.T, db *pgxpool.Pool, lookup lookupFunc) *Service {
	t.Helper()
	if db == nil {
		db = pgtest.Open(t, storage.Migrations())
	}
	return &Service{db: db, q: dbgen.New(db), client: lookup, logger: slog.New(slog.DiscardHandler)}
}

func importGallery(t *testing.T, s *Service, pandaID int64) int64 {
	t.Helper()
	tx, err := s.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err := tx.Exec(t.Context(), `INSERT INTO libraries (id, name, path) VALUES (1, 'Library', '/library') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	var next int64
	if err := tx.QueryRow(t.Context(), `SELECT count(*) + 1 FROM sources`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("Source %d [%d]", next, pandaID)
	imported, err := source.NewStore(s.db).CreateTx(t.Context(), tx, 1, name, source.Directory, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := gallery.NewStore(s.db).CreateFromSourceTx(t.Context(), tx, imported.ID, metadata.Values{Title: "Snapshot", Tags: []tag.Value{{Namespace: "other", Value: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueTx(t.Context(), tx, g.ID, name, source.Directory); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func pendingCount(t *testing.T, db *pgxpool.Pool, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(t.Context(), `SELECT count(*) FROM panda_enrichments`).Scan(&count); err != nil || count != want {
		t.Fatalf("pending=%d, want=%d, error=%v", count, want, err)
	}
}

func checkGallery(t *testing.T, s *Service, id int64, title string, values ...string) {
	t.Helper()
	g, err := gallery.NewStore(s.db).Get(t.Context(), id)
	if err != nil || g.Title != title {
		t.Fatalf("gallery=%+v, want title=%q, error=%v", g, title, err)
	}
	tags, err := tag.NewStore(s.db).ListForGallery(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, value := range tags {
		got = append(got, value.Namespace.Name+":"+value.Value)
	}
	if !reflect.DeepEqual(got, values) {
		t.Fatalf("tags=%v, want=%v", got, values)
	}
}

func step(t *testing.T, s *Service, at time.Time) {
	t.Helper()
	if _, err := s.step(t.Context(), at); err != nil {
		t.Fatal(err)
	}
}

func TestEnrichmentResumesAfterOutageAndRestart(t *testing.T) {
	at := time.Now()
	calls := 0
	lookup := lookupFunc(func(ctx context.Context, ids []int64) (collectorapi.LookupResult, error) {
		calls++
		if calls <= 2 && !reflect.DeepEqual(ids, []int64{1, 2, 3, 4, 5}) {
			t.Fatalf("batch not deduplicated: %v", ids)
		}
		if calls == 1 {
			return collectorapi.LookupResult{}, errors.New("collector offline")
		}
		if calls == 2 {
			return collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{
				{Metadata: panda.Metadata{ID: 1, Title: "API", Tags: []string{"Language:English", "bad_tag"}}, RefreshedAt: time.Unix(1, 0)},
				{Metadata: panda.Metadata{ID: 5}},
			}, PendingIDs: []int64{2}, FailedIDs: []int64{3}, UnknownIDs: []int64{4}}, nil
		}
		if !reflect.DeepEqual(ids, []int64{2}) {
			t.Fatalf("completed work retried: %v", ids)
		}
		return collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{{Metadata: panda.Metadata{ID: 2, TitleJapanese: "原題"}}}}, nil
	})
	s := openService(t, nil, lookup)
	var galleries []int64
	for _, id := range []int64{1, 2, 3, 4, 1, 5} {
		galleries = append(galleries, importGallery(t, s, id))
	}
	step(t, s, at)
	pendingCount(t, s.db, 6)
	checkGallery(t, s, galleries[0], "Snapshot", "other:snapshot")
	s = openService(t, pgtest.Reopen(t, s.db, storage.Migrations()), lookup)
	step(t, s, at.Add(59*time.Second))
	if calls != 1 {
		t.Fatalf("restart bypassed retry delay: %d", calls)
	}
	step(t, s, at.Add(time.Minute))
	pendingCount(t, s.db, 1)
	for _, id := range []int64{galleries[0], galleries[4]} {
		checkGallery(t, s, id, "API", "language:english")
	}
	for _, id := range galleries[1:4] {
		checkGallery(t, s, id, "Snapshot", "other:snapshot")
	}
	checkGallery(t, s, galleries[5], "Snapshot")
	if _, err := gallery.NewStore(s.db).Rename(t.Context(), galleries[1], "User edit"); err != nil {
		t.Fatal(err)
	}
	step(t, s, at.Add(2*time.Minute))
	pendingCount(t, s.db, 0)
	checkGallery(t, s, galleries[1], "原題")
}

func TestMetadataAndCompletionCommitTogetherAndDeletedGalleriesStayDeleted(t *testing.T) {
	var s *Service
	var id int64
	deleteDuringLookup := false
	s = openService(t, nil, func(ctx context.Context, _ []int64) (collectorapi.LookupResult, error) {
		if deleteDuringLookup {
			if _, err := s.db.Exec(ctx, `DELETE FROM galleries WHERE id = $1`, id); err != nil {
				t.Fatal(err)
			}
		}
		return collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{{Metadata: panda.Metadata{ID: 1, Title: "API", Tags: []string{"language:english"}}}}}, nil
	})
	id = importGallery(t, s, 1)
	if _, err := s.db.Exec(t.Context(), `CREATE FUNCTION fail_completion() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test failure'; END $$;
		CREATE TRIGGER fail_completion BEFORE DELETE ON panda_enrichments FOR EACH ROW EXECUTE FUNCTION fail_completion()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.step(t.Context(), time.Now()); err == nil {
		t.Fatal("expected completion failure")
	}
	pendingCount(t, s.db, 1)
	checkGallery(t, s, id, "Snapshot", "other:snapshot")
	if _, err := s.db.Exec(t.Context(), `DROP TRIGGER fail_completion ON panda_enrichments`); err != nil {
		t.Fatal(err)
	}
	step(t, s, time.Now())
	pendingCount(t, s.db, 0)
	checkGallery(t, s, id, "API", "language:english")
	id = importGallery(t, s, 1)
	deleteDuringLookup = true
	step(t, s, time.Now())
	pendingCount(t, s.db, 0)
	if _, err := gallery.NewStore(s.db).Get(t.Context(), id); !errors.Is(err, gallery.ErrNotFound) {
		t.Fatalf("gallery recreated: %v", err)
	}
}
