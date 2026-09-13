package cleanup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/cleanup/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type lookupStub struct {
	values map[int64]panda.Metadata
	calls  int
}

func (l *lookupStub) Lookup(_ context.Context, ids []int64) (collectorapi.LookupResult, error) {
	l.calls++
	if len(ids) > collectorapi.MaxLookupSize {
		return collectorapi.LookupResult{}, errors.New("oversized lookup")
	}
	result := collectorapi.LookupResult{}
	for _, id := range ids {
		if value, ok := l.values[id]; ok {
			result.Galleries = append(result.Galleries, collectorapi.CollectedMetadata{Metadata: value})
		} else {
			result.UnknownIDs = append(result.UnknownIDs, id)
		}
	}
	return result, nil
}

func metadata(id, parent int64) panda.Metadata {
	v := panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id), Title: fmt.Sprintf("Title &amp; %d", id), ParentID: parent}
	if parent != 0 {
		v.ParentToken = fmt.Sprintf("token%d", parent)
	}
	return v
}

type fixture struct {
	db      *sql.DB
	service *Service
	lookup  *lookupStub
}

func newFixture(t *testing.T, values ...panda.Metadata) fixture {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	l := &lookupStub{values: map[int64]panda.Metadata{}}
	for _, value := range values {
		l.values[value.ID] = value
	}
	return fixture{db: db, service: New(db, l), lookup: l}
}

func (f fixture) addSource(t *testing.T, name string, kind source.Kind) (source.Source, string, int64) {
	t.Helper()
	root := t.TempDir()
	if name == "." {
		root = filepath.Join(root, "Newest [3]")
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	l, err := library.NewSQLiteRepository(f.db).Create(t.Context(), name, root)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, name)
	if kind == source.Archive {
		// Deliberately not an archive: cleanup only checks presence and type.
		err = os.WriteFile(path, []byte("present"), 0o600)
	} else {
		err = os.MkdirAll(path, 0o700)
	}
	if err != nil {
		t.Fatal(err)
	}
	s, err := source.NewSQLiteRepository(f.db).Create(t.Context(), l.ID, name, kind, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	g, err := gallery.NewSQLiteRepository(f.db).CreateFromSource(t.Context(), s.ID)
	if err != nil {
		t.Fatal(err)
	}
	return s, path, g.ID
}

func TestCleanupDeletesGlobalChainAndKeepsSurvivingReplacement(t *testing.T) {
	f := newFixture(t, metadata(1, 0), metadata(2, 1), metadata(3, 2))
	a, ap, ag := f.addSource(t, "Old [1].cbz", source.Archive)
	b, bp, bg := f.addSource(t, "Middle [2].zip", source.Archive)
	c, cp, _ := f.addSource(t, "New [3].cbz", source.Archive)
	preview, err := f.service.Preview(t.Context())
	if err != nil || len(preview.Candidates) != 2 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	for _, item := range preview.Candidates {
		if item.Replacement.ID != c.ID || item.Replacement.Path != cp || item.SizeBytes != 7 || item.Replacement.Title != "Title & 3" {
			t.Fatalf("candidate = %+v", item)
		}
	}
	calls := f.lookup.calls
	result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{b.ID, a.ID})
	if err != nil || !reflect.DeepEqual(result.Deleted, []int64{b.ID, a.ID}) || len(result.Failed) != 0 {
		t.Fatalf("execute = %+v, %v", result, err)
	}
	if f.lookup.calls != calls {
		t.Fatal("execution queried collector")
	}
	for _, path := range []string{ap, bp} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old archive retained: %s, %v", path, err)
		}
	}
	for _, id := range []int64{ag, bg} {
		if _, err := gallery.NewSQLiteRepository(f.db).Get(t.Context(), id); !errors.Is(err, gallery.ErrNotFound) {
			t.Fatalf("linked gallery retained: %d, %v", id, err)
		}
	}
	if _, err := os.Stat(cp); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID}); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("replayed plan: %v", err)
	}
}

func TestCleanupRechecksPresenceAndIndependentGalleryReferences(t *testing.T) {
	for _, change := range []string{"missing replacement", "changed archive", "independent gallery", "duplicate association"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t, metadata(1, 0), metadata(2, 1))
			a, ap, _ := f.addSource(t, "Old [1].cbz", source.Archive)
			_, cp, _ := f.addSource(t, "New [2].cbz", source.Archive)
			preview, err := f.service.Preview(t.Context())
			if err != nil || len(preview.Candidates) != 1 {
				t.Fatalf("preview = %+v, %v", preview, err)
			}
			switch change {
			case "missing replacement":
				err = os.Remove(cp)
			case "changed archive":
				err = os.WriteFile(ap, []byte("changed since review"), 0o600)
			case "independent gallery":
				files, filesErr := source.NewSQLiteRepository(f.db).Files(t.Context(), a.ID)
				if filesErr != nil {
					t.Fatal(filesErr)
				}
				_, err = gallery.NewSQLiteRepository(f.db).Create(t.Context(), "Keep", []int64{files[0].ID})
			case "duplicate association":
				f.addSource(t, "Another [1].zip", source.Archive)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID})
			if err != nil || len(result.Deleted) != 0 || len(result.Failed) != 1 {
				t.Fatalf("execute = %+v, %v", result, err)
			}
			if _, err := os.Stat(ap); err != nil {
				t.Fatalf("old archive lost: %v", err)
			}
			if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), a.ID); err != nil {
				t.Fatalf("old source lost: %v", err)
			}
			if change == "independent gallery" {
				next, err := f.service.Preview(t.Context())
				if err != nil || len(next.Candidates) != 0 {
					t.Fatalf("protected source offered: %+v, %v", next, err)
				}
			}
		})
	}
}

func TestCleanupStalledGlobalProbeDoesNotBlockCatalog(t *testing.T) {
	for _, stop := range []string{"timeout", "cancellation"} {
		t.Run(stop, func(t *testing.T) {
			f := newFixture(t, metadata(1, 0), metadata(2, 1))
			a, ap, ag := f.addSource(t, "Old [1].cbz", source.Archive)
			f.addSource(t, "New [2].cbz", source.Archive)
			unrelated, _, _ := f.addSource(t, "Unrelated [3].cbz", source.Archive)
			preview, err := f.service.Preview(t.Context())
			if err != nil || len(preview.Candidates) != 1 {
				t.Fatalf("preview = %+v, %v", preview, err)
			}
			started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			f.service.canonicalPath = func(row dbgen.ListSourcesRow) (string, error) {
				if row.ID == unrelated.ID {
					close(started)
					<-release
					defer close(finished)
				}
				return filepathCanonical(row)
			}
			f.service.probeTimeout = 100 * time.Millisecond
			if stop == "cancellation" {
				f.service.probeTimeout = 5 * time.Second
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			type outcome struct {
				result Result
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := f.service.Execute(ctx, preview.PlanID, []int64{a.ID})
				done <- outcome{result, err}
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("global probe did not start")
			}
			readCtx, readCancel := context.WithTimeout(t.Context(), time.Second)
			defer readCancel()
			if _, err := gallery.NewSQLiteRepository(f.db).Get(readCtx, ag); err != nil {
				t.Fatalf("stalled global probe blocked gallery read: %v", err)
			}
			wantErr := context.DeadlineExceeded
			if stop == "cancellation" {
				cancel()
				wantErr = context.Canceled
			}
			select {
			case got := <-done:
				if !errors.Is(got.err, wantErr) || len(got.result.Deleted) != 0 {
					t.Fatalf("execute = %+v, %v; want %v", got.result, got.err, wantErr)
				}
			case <-time.After(time.Second):
				t.Fatal("cleanup kept waiting for stalled filesystem")
			}
			unblock()
			<-finished
			if _, err := os.Stat(ap); err != nil {
				t.Fatalf("old archive lost: %v", err)
			}
			if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), a.ID); err != nil {
				t.Fatalf("old source lost after late probe result: %v", err)
			}
		})
	}
}

func TestCleanupRevalidatesCatalogAfterGlobalProbe(t *testing.T) {
	for _, change := range []string{"added alias", "changed path", "independent gallery"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t, metadata(1, 0), metadata(2, 1))
			a, ap, _ := f.addSource(t, "Old [1].cbz", source.Archive)
			f.addSource(t, "New [2].cbz", source.Archive)
			unrelated, up, _ := f.addSource(t, "Unrelated [3].cbz", source.Archive)
			preview, err := f.service.Preview(t.Context())
			if err != nil || len(preview.Candidates) != 1 {
				t.Fatalf("preview = %+v, %v", preview, err)
			}
			alias := filepath.Join(filepath.Dir(up), "Alias.cbz")
			if err := os.Symlink(ap, alias); err != nil {
				t.Fatal(err)
			}
			files, err := source.NewSQLiteRepository(f.db).Files(t.Context(), a.ID)
			if err != nil {
				t.Fatal(err)
			}
			changed := make(chan error, 1)
			f.service.canonicalPath = func(row dbgen.ListSourcesRow) (string, error) {
				path, err := filepathCanonical(row)
				if row.ID == unrelated.ID {
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					var changeErr error
					switch change {
					case "added alias":
						_, changeErr = source.NewSQLiteRepository(f.db).Create(ctx, unrelated.LibraryID, filepath.Base(alias), source.Archive, []string{"1.jpg"})
					case "changed path":
						_, changeErr = f.db.ExecContext(ctx, "UPDATE sources SET path = ? WHERE id = ?", filepath.Base(alias), unrelated.ID)
					case "independent gallery":
						_, changeErr = gallery.NewSQLiteRepository(f.db).Create(ctx, "Keep", []int64{files[0].ID})
					}
					changed <- changeErr
				}
				return path, err
			}
			result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID})
			if changeErr := <-changed; changeErr != nil {
				t.Fatalf("change during global probe: %v", changeErr)
			}
			if change == "independent gallery" {
				if err != nil || len(result.Failed) != 1 || result.Failed[0].Reason != "Source is now used by an independent gallery" {
					t.Fatalf("execute = %+v, %v", result, err)
				}
			} else if !errors.Is(err, ErrInvalidSelection) {
				t.Fatalf("execute = %+v, %v; want invalidated review", result, err)
			}
			if len(result.Deleted) != 0 {
				t.Fatalf("deleted source after catalog change: %+v", result)
			}
			if _, err := os.Stat(ap); err != nil {
				t.Fatalf("old archive lost: %v", err)
			}
			if _, err := source.NewSQLiteRepository(f.db).Get(t.Context(), a.ID); err != nil {
				t.Fatalf("old source lost: %v", err)
			}
		})
	}
}

func TestCleanupPartialFailureDoesNotStopOtherCandidates(t *testing.T) {
	f := newFixture(t, metadata(1, 0), metadata(2, 1), metadata(3, 2))
	a, _, _ := f.addSource(t, "Old [1].cbz", source.Archive)
	b, bp, _ := f.addSource(t, "Middle [2].cbz", source.Archive)
	f.addSource(t, "Newest [3].cbz", source.Archive)
	preview, err := f.service.Preview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bp, []byte("changed since review"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{b.ID, a.ID})
	if err != nil || !reflect.DeepEqual(result.Deleted, []int64{a.ID}) || len(result.Failed) != 1 || result.Failed[0].SourceID != b.ID {
		t.Fatalf("execute = %+v, %v", result, err)
	}
}

func TestCleanupRequiresProvenAncestryAndUnambiguousFiles(t *testing.T) {
	for _, problem := range []string{"missing intermediate", "cycle", "conflicting token", "duplicate Panda ID", "symlink alias", "same file"} {
		t.Run(problem, func(t *testing.T) {
			f := newFixture(t, metadata(1, 0), metadata(3, 1))
			a, ap, _ := f.addSource(t, "Old [1].cbz", source.Archive)
			_, cp, _ := f.addSource(t, "Newest [3].cbz", source.Archive)
			switch problem {
			case "missing intermediate":
				f.lookup.values[3] = metadata(3, 2)
			case "cycle":
				f.lookup.values[1] = metadata(1, 3)
			case "conflicting token":
				bad := metadata(3, 1)
				bad.ParentToken = "different"
				f.lookup.values[3] = bad
			case "duplicate Panda ID":
				f.addSource(t, "Duplicate [1].cbz", source.Archive)
			case "symlink alias":
				alias := filepath.Join(filepath.Dir(ap), "Alias [4].cbz")
				if err := os.Symlink(filepath.Base(ap), alias); err != nil {
					t.Fatal(err)
				}
				if _, err := source.NewSQLiteRepository(f.db).Create(t.Context(), a.LibraryID, filepath.Base(alias), source.Archive, []string{"1.jpg"}); err != nil {
					t.Fatal(err)
				}
			case "same file":
				if err := os.Remove(cp); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(ap, cp); err != nil {
					t.Fatal(err)
				}
			}
			preview, err := f.service.Preview(t.Context())
			if err != nil || len(preview.Candidates) != 0 {
				t.Fatalf("unsafe preview = %+v, %v", preview, err)
			}
		})
	}
}

func TestCleanupAcceptsKnownSegmentAndRootDirectoryReplacement(t *testing.T) {
	// ID 1's own metadata is absent. The known 3 -> 2 -> 1 segment proves it
	// older without a locally imported intermediate or history before ID 1.
	f := newFixture(t, metadata(2, 1), metadata(3, 2))
	a, _, _ := f.addSource(t, "Old [1].cbz", source.Archive)
	c, cp, _ := f.addSource(t, ".", source.Directory)
	preview, err := f.service.Preview(t.Context())
	if err != nil || len(preview.Candidates) != 1 || preview.Candidates[0].Replacement.ID != c.ID || preview.Candidates[0].Replacement.PandaID != 3 {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	result, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID})
	if err != nil || len(result.Deleted) != 1 {
		t.Fatalf("execute = %+v, %v", result, err)
	}
	if info, err := os.Stat(cp); err != nil || !info.IsDir() {
		t.Fatalf("replacement directory lost: %v", err)
	}
}

func TestCleanupRejectsUnreviewedOrDuplicateSelection(t *testing.T) {
	f := newFixture(t, metadata(1, 0), metadata(2, 1))
	a, ap, _ := f.addSource(t, "Old [1].cbz", source.Archive)
	c, _, _ := f.addSource(t, "New [2].cbz", source.Archive)
	preview, err := f.service.Preview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, selection := range [][]int64{nil, {c.ID}, {a.ID, a.ID}, {a.ID, c.ID}} {
		if _, err := f.service.Execute(t.Context(), preview.PlanID, selection); !errors.Is(err, ErrInvalidSelection) {
			t.Fatalf("selection %v = %v", selection, err)
		}
	}
	if _, err := os.Stat(ap); err != nil {
		t.Fatal(err)
	}
	f.service.plans[preview.PlanID] = plan{created: f.service.plans[preview.PlanID].created.Add(-31 * time.Minute)}
	if _, err := f.service.Execute(t.Context(), preview.PlanID, []int64{a.ID}); !errors.Is(err, ErrInvalidSelection) || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired plan: %v", err)
	}
}
