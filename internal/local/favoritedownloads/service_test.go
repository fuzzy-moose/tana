package favoritedownloads

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/panda"
)

type fakeCollector struct {
	favorites   []collectorapi.FavoriteDownloadCandidate
	category    int
	reads       int
	submissions [][]panda.GalleryRef
	counts      collectorapi.DownloadBatchCounts
	err         error
}

func (c *fakeCollector) FavoriteDownloadCandidates(_ context.Context, category int) ([]collectorapi.FavoriteDownloadCandidate, error) {
	c.category = category
	c.reads++
	return c.favorites, nil
}

func (c *fakeCollector) SubmitDownloads(_ context.Context, refs []panda.GalleryRef) (collectorapi.DownloadBatchCounts, error) {
	c.submissions = append(c.submissions, append([]panda.GalleryRef(nil), refs...))
	return c.counts, c.err
}

func fixture(t *testing.T, client *fakeCollector) (*Service, *sql.DB) {
	t.Helper()
	db, _, err := storage.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db, client), db
}

func addLibrary(t *testing.T, db *sql.DB, root string) library.Library {
	t.Helper()
	lib, err := library.NewSQLiteRepository(db).Create(t.Context(), "Library", root)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

func addSource(t *testing.T, db *sql.DB, libraryID int64, name string, kind source.Kind) source.Source {
	t.Helper()
	src, err := source.NewSQLiteRepository(db).Create(t.Context(), libraryID, name, kind, []string{"1.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func favorite(id int64, state string) collectorapi.FavoriteDownloadCandidate {
	return collectorapi.FavoriteDownloadCandidate{Ref: panda.GalleryRef{ID: id, Token: "token"}, State: state}
}

func TestPreviewUsesExactCatalogIDsAcrossOfflineLibraries(t *testing.T) {
	client := &fakeCollector{favorites: []collectorapi.FavoriteDownloadCandidate{
		favorite(1, ""), favorite(2, ""), favorite(3, ""), favorite(4, ""),
		favorite(5, "queued"), favorite(6, "running"), favorite(7, "completed"),
		favorite(8, "failed"), favorite(9, "cancelled"), favorite(10, "deleting"),
	}}
	s, db := fixture(t, client)
	// Catalog entries intentionally have no files or directories on disk.
	first := addLibrary(t, db, filepath.Join(t.TempDir(), "offline"))
	second := addLibrary(t, db, filepath.Join(t.TempDir(), "Root [3]"))
	addSource(t, db, first.ID, "nested/Title [1].CBZ", source.Archive)
	addSource(t, db, first.ID, "Title [2]", source.Directory)
	addSource(t, db, second.ID, ".", source.Directory)
	addSource(t, db, first.ID, "Title [4] extra.cbz", source.Archive)
	addSource(t, db, first.ID, "Different version [44].cbz", source.Archive)
	if err := library.NewSQLiteRepository(db).UpdateAvailability(t.Context(), first.ID, "unavailable", time.Now()); err != nil {
		t.Fatal(err)
	}
	preview, err := s.Preview(t.Context(), 6)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Category: 6, Total: 10, Present: 3, Missing: 7, DownloadBatchCounts: collectorapi.DownloadBatchCounts{
		NewDownloads: 1, ExistingJobs: 2, RetainedArchives: 1, Failed: 1, Cancelled: 1, Deleting: 1,
	}}
	if preview.PlanID == "" || preview.Result != want || client.category != 6 || len(client.submissions) != 0 {
		t.Fatalf("preview: %+v, submissions: %v", preview, client.submissions)
	}
}

func TestConfirmationPreservesReviewedScopeAndCanBeReplayed(t *testing.T) {
	client := &fakeCollector{favorites: []collectorapi.FavoriteDownloadCandidate{favorite(1, ""), favorite(2, ""), favorite(3, "")}}
	s, db := fixture(t, client)
	lib := addLibrary(t, db, filepath.Join(t.TempDir(), "offline"))
	previouslyPresent := addSource(t, db, lib.ID, "Title [1].cbz", source.Archive)
	preview, err := s.Preview(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	// A removed source must not expand the review; a newly imported source
	// must be excluded. Membership changes never trigger another collection read.
	if err := source.NewSQLiteRepository(db).Delete(t.Context(), previouslyPresent.ID); err != nil {
		t.Fatal(err)
	}
	addSource(t, db, lib.ID, "Title [2].zip", source.Archive)
	client.favorites = []collectorapi.FavoriteDownloadCandidate{favorite(4, "")}
	client.counts = collectorapi.DownloadBatchCounts{ExistingJobs: 1}
	client.err = errors.New("response lost")
	if _, err := s.Execute(t.Context(), 2, preview.PlanID); !errors.Is(err, client.err) {
		t.Fatalf("expected uncertain response, got %v", err)
	}
	client.err = nil
	result, err := s.Execute(t.Context(), 2, preview.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	want := Result{Category: 2, Total: 3, Present: 2, Missing: 1, DownloadBatchCounts: client.counts}
	if result != want || client.reads != 1 || len(client.submissions) != 2 {
		t.Fatalf("confirmation: %+v, reads: %d, submissions: %v", result, client.reads, client.submissions)
	}
	for _, refs := range client.submissions {
		if !reflect.DeepEqual(refs, []panda.GalleryRef{favorite(3, "").Ref}) {
			t.Fatalf("scope changed: %+v", refs)
		}
	}
	replayed, err := s.Execute(t.Context(), 2, preview.PlanID)
	if err != nil || replayed != result || len(client.submissions) != 2 {
		t.Fatalf("replay: %+v, %v", replayed, err)
	}
}

func TestConfirmationPreservesSkippedJobsUntilFreshPreview(t *testing.T) {
	for _, state := range []string{"failed", "cancelled", "deleting"} {
		t.Run(state, func(t *testing.T) {
			client := &fakeCollector{favorites: []collectorapi.FavoriteDownloadCandidate{favorite(1, state)}}
			s, _ := fixture(t, client)
			preview, err := s.Preview(t.Context(), 2)
			if err != nil {
				t.Fatal(err)
			}
			if preview.NewDownloads != 0 || preview.Missing != 1 {
				t.Fatalf("preview: %+v", preview)
			}
			// Deletion finishes before confirmation; submitting this reference
			// would now create a new download.
			client.favorites = []collectorapi.FavoriteDownloadCandidate{favorite(1, "")}
			client.counts = collectorapi.DownloadBatchCounts{NewDownloads: 1}
			for range 2 {
				result, err := s.Execute(t.Context(), 2, preview.PlanID)
				if err != nil || result != preview.Result || len(client.submissions) != 0 {
					t.Fatalf("skipped job submitted: result=%+v, submissions=%v, err=%v", result, client.submissions, err)
				}
			}
			fresh, err := s.Preview(t.Context(), 2)
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.Execute(t.Context(), 2, fresh.PlanID)
			if err != nil || fresh.NewDownloads != 1 || result != fresh.Result || len(client.submissions) != 1 {
				t.Fatalf("fresh preview: %+v, result=%+v, submissions=%v, err=%v", fresh, result, client.submissions, err)
			}
		})
	}
}

func TestConfirmationCombinesSkippedJobsWithBatchAndRechecksPresence(t *testing.T) {
	client := &fakeCollector{favorites: []collectorapi.FavoriteDownloadCandidate{
		favorite(1, "failed"), favorite(2, "cancelled"), favorite(3, "deleting"),
		favorite(4, "deleting"), favorite(5, ""), favorite(6, ""),
	}}
	s, db := fixture(t, client)
	lib := addLibrary(t, db, filepath.Join(t.TempDir(), "offline"))
	preview, err := s.Preview(t.Context(), 2)
	if err != nil {
		t.Fatal(err)
	}
	addSource(t, db, lib.ID, "Title [4].cbz", source.Archive)
	client.counts = collectorapi.DownloadBatchCounts{NewDownloads: 1, Deleting: 1}
	result, err := s.Execute(t.Context(), 2, preview.PlanID)
	want := Result{Category: 2, Total: 6, Present: 1, Missing: 5, DownloadBatchCounts: collectorapi.DownloadBatchCounts{
		NewDownloads: 1, Failed: 1, Cancelled: 1, Deleting: 2,
	}}
	if err != nil || result != want {
		t.Fatalf("confirmation: %+v, %v; want %+v", result, err, want)
	}
	if !reflect.DeepEqual(client.submissions, [][]panda.GalleryRef{{favorite(5, "").Ref, favorite(6, "").Ref}}) {
		t.Fatalf("skipped jobs submitted: %+v", client.submissions)
	}
}

func TestPreviewMustBeCurrentAndMatchCategory(t *testing.T) {
	client := &fakeCollector{}
	s, _ := fixture(t, client)
	preview, err := s.Preview(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(t.Context(), 1, preview.PlanID); !errors.Is(err, ErrInvalidPreview) {
		t.Fatalf("wrong category: %v", err)
	}
	result, err := s.Execute(t.Context(), 0, preview.PlanID)
	if err != nil || result != (Result{}) || len(client.submissions) != 0 {
		t.Fatalf("empty category: %+v, %v", result, err)
	}
	s.plans[preview.PlanID].created = time.Now().Add(-previewLifetime - time.Second)
	if _, err := s.Execute(t.Context(), 0, preview.PlanID); !errors.Is(err, ErrInvalidPreview) {
		t.Fatalf("expired preview: %v", err)
	}
	if _, err := s.Execute(t.Context(), 0, "unknown"); !errors.Is(err, ErrInvalidPreview) {
		t.Fatalf("unknown preview: %v", err)
	}
	if _, err := s.Preview(t.Context(), 10); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("invalid category: %v", err)
	}
}
