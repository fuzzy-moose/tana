package enrichment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/enrichment/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/local/tag"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type lookupFunc func(context.Context, []int64) (collectorapi.LookupResult, error)

func (f lookupFunc) Lookup(ctx context.Context, ids []int64) (collectorapi.LookupResult, error) {
	return f(ctx, ids)
}

func openService(t *testing.T, dir string, lookup lookupFunc) *Service {
	t.Helper()
	db, _, err := storage.Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Service{db: db, q: dbgen.New(db), client: lookup, logger: slog.New(slog.DiscardHandler)}
}

func importGallery(t *testing.T, s *Service, pandaID int64) int64 {
	t.Helper()
	tx, err := s.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO libraries (id, name, path) VALUES (1, 'Library', '/library') ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	var next int64
	if err := tx.QueryRow(`SELECT count(*) + 1 FROM sources`).Scan(&next); err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("Source %d [%d]", next, pandaID)
	imported, err := source.NewSQLiteRepository(s.db).CreateTx(t.Context(), tx, 1, name, source.Directory, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := gallery.NewSQLiteRepository(s.db).CreateFromSourceTx(t.Context(), tx, imported.ID, metadata.Values{Title: "Snapshot", Tags: []tag.Value{{Namespace: "other", Value: "snapshot"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnqueueTx(t.Context(), tx, g.ID, name, source.Directory); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return g.ID
}

func pendingCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM panda_enrichments`).Scan(&count); err != nil || count != want {
		t.Fatalf("pending=%d, want=%d, error=%v", count, want, err)
	}
}

func checkGallery(t *testing.T, s *Service, id int64, title string, values ...string) {
	t.Helper()
	g, err := gallery.NewSQLiteRepository(s.db).Get(t.Context(), id)
	if err != nil || g.Title != title {
		t.Fatalf("gallery=%+v, want title=%q, error=%v", g, title, err)
	}
	tags, err := tag.NewSQLiteRepository(s.db).ListForGallery(t.Context(), id)
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
	dir := t.TempDir()
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
	s := openService(t, dir, lookup)
	var galleries []int64
	for _, id := range []int64{1, 2, 3, 4, 1, 5} {
		galleries = append(galleries, importGallery(t, s, id))
	}
	step(t, s, at)
	pendingCount(t, s.db, 6)
	checkGallery(t, s, galleries[0], "Snapshot", "other:snapshot")
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s = openService(t, dir, lookup)
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
	if _, err := gallery.NewSQLiteRepository(s.db).Rename(t.Context(), galleries[1], "User edit"); err != nil {
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
	s = openService(t, t.TempDir(), func(context.Context, []int64) (collectorapi.LookupResult, error) {
		if deleteDuringLookup {
			if _, err := s.db.Exec(`DELETE FROM galleries WHERE id = ?`, id); err != nil {
				t.Fatal(err)
			}
		}
		return collectorapi.LookupResult{Galleries: []collectorapi.CollectedMetadata{{Metadata: panda.Metadata{ID: 1, Title: "API", Tags: []string{"language:english"}}}}}, nil
	})
	id = importGallery(t, s, 1)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_completion BEFORE DELETE ON panda_enrichments BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.step(t.Context(), time.Now()); err == nil {
		t.Fatal("expected completion failure")
	}
	pendingCount(t, s.db, 1)
	checkGallery(t, s, id, "Snapshot", "other:snapshot")
	if _, err := s.db.Exec(`DROP TRIGGER fail_completion`); err != nil {
		t.Fatal(err)
	}
	step(t, s, time.Now())
	pendingCount(t, s.db, 0)
	checkGallery(t, s, id, "API", "language:english")
	id = importGallery(t, s, 1)
	deleteDuringLookup = true
	step(t, s, time.Now())
	pendingCount(t, s.db, 0)
	if _, err := gallery.NewSQLiteRepository(s.db).Get(t.Context(), id); !errors.Is(err, gallery.ErrNotFound) {
		t.Fatalf("gallery recreated: %v", err)
	}
}
