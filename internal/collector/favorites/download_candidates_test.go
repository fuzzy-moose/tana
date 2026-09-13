package favorites

import (
	"errors"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collector/favorites/dbgen"
)

func TestDownloadCandidatesIncludeWholeCategoryForCurrentAccount(t *testing.T) {
	s := testStore(t)
	for _, scope := range []dbgen.SaveCategoryParams{
		{Host: s.host, AccountKey: s.accountKey, Category: 2},
		{Host: s.host, AccountKey: s.accountKey, Category: 3},
		{Host: s.host, AccountKey: "other", Category: 2},
		{Host: "https://other.test", AccountKey: s.accountKey, Category: 2},
	} {
		categoryID, err := s.q.SaveCategory(t.Context(), scope)
		if err != nil {
			t.Fatal(err)
		}
		for id := int64(1); id <= 125; id++ {
			if err := s.q.SaveGalleryRef(t.Context(), dbgen.SaveGalleryRefParams{GalleryID: categoryID*1000 + id, Token: "token"}); err != nil {
				t.Fatal(err)
			}
			if err := s.q.SaveFavorite(t.Context(), dbgen.SaveFavoriteParams{CategoryID: categoryID, GalleryID: categoryID*1000 + id, Token: "token", AddedAt: id}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := s.db.Exec(`INSERT INTO panda_downloads(gallery_id, token, state, created_at, updated_at) VALUES
		(1001, 'token', 'completed', 1, 1), (1002, 'token', 'failed', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	service := &Service{store: s}
	got, err := service.DownloadCandidates(t.Context(), 2)
	if err != nil || len(got) != 125 {
		t.Fatalf("candidates=%d %v", len(got), err)
	}
	for i, candidate := range got {
		state := ""
		if i == 0 {
			state = "completed"
		} else if i == 1 {
			state = "failed"
		}
		if candidate.Ref.ID != int64(1001+i) || candidate.Ref.Token != "token" || candidate.State != state {
			t.Fatalf("candidate %d=%+v", i, candidate)
		}
	}
	if got, err := service.DownloadCandidates(t.Context(), 0); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty category=%+v %v", got, err)
	}
	if _, err := service.DownloadCandidates(t.Context(), 10); !errors.Is(err, ErrInvalidCategory) {
		t.Fatalf("invalid category=%v", err)
	}
}
