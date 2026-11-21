package catalog

import (
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func listGraded(t *testing.T, s *Service) collectorapi.CatalogResult {
	t.Helper()
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// assertGrade checks the score grade of a listed gallery and the number of
// held-out favorite scores above its score.
func assertGrade(t *testing.T, result collectorapi.CatalogResult, id int64, grade string, above int) {
	t.Helper()
	for _, item := range result.Items {
		if item.GalleryID != id {
			continue
		}
		if item.Score == nil || item.Score.Grade != grade || item.Score.FavoritesAbove == nil || *item.Score.FavoritesAbove != above {
			t.Fatalf("gallery %d score = %+v, want grade %q with %d above", id, item.Score, grade, above)
		}
		return
	}
	t.Fatalf("gallery %d not listed", id)
}

func TestScoreGradesCompareWithTheHeldOutFavoriteScores(t *testing.T) {
	db, s := openCatalog(t)
	// Each tag is on one favorite only, so it has no calculated weight: the
	// pins alone give favorites 1 to 4 the held-out scores 1 to 4.
	for id, tag := range map[int64]string{1: "other:a1", 2: "other:a2", 3: "other:a3", 4: "other:a4", 5: "other:a4", 6: "other:a3", 7: "other:a2"} {
		retain(t, db, panda.Metadata{ID: id, Posted: id, Tags: []string{tag}}, true)
	}
	retain(t, db, panda.Metadata{ID: 8, Posted: 8}, true)
	favor(t, db, "42", 0, 1, 2, 3, 4)
	for i, tag := range []string{"other:a1", "other:a2", "other:a3", "other:a4"} {
		setPin(t, s, collectorapi.ScoreKindTag, tag, float64(i+1))
	}

	// No model: no grade, even while an override puts the filter in force.
	setOverride(t, s, collectorapi.ScoreKindTag, "other:a1", collectorapi.ScoreRuleNeverHide)
	result := listGraded(t, s)
	for _, item := range result.Items {
		if item.Score != nil && (item.Score.Grade != "" || item.Score.FavoritesAbove != nil) {
			t.Fatalf("gallery %d is graded without a model: %+v", item.GalleryID, item.Score)
		}
	}
	if _, err := db.Exec(t.Context(), "DELETE FROM panda_score_overrides"); err != nil {
		t.Fatal(err)
	}

	candidate := previewScore(t, s).Candidate
	if err := s.ApplyScoreModel(t.Context(), candidate.ID, -100); err != nil {
		t.Fatal(err)
	}
	result = listGraded(t, s)
	if result.ScoreFilter == nil || result.ScoreFilter.HeldOutFavorites != 4 {
		t.Fatalf("score filter = %+v", result.ScoreFilter)
	}
	// A score on a cut point gets the higher grade.
	assertGrade(t, result, 5, collectorapi.ScoreGradeTopQuarter, 0)
	assertGrade(t, result, 6, collectorapi.ScoreGradeTopHalf, 1)
	assertGrade(t, result, 7, "", 2)
	assertGrade(t, result, 8, "", 4)

	// The grades follow a pin: the favorite scores become 0.5, 1, 2 and 3.
	setPin(t, s, collectorapi.ScoreKindTag, "other:a4", 0.5)
	result = listGraded(t, s)
	assertGrade(t, result, 5, "", 3)
	assertGrade(t, result, 6, collectorapi.ScoreGradeTopQuarter, 0)

	// The threshold does not change the grades.
	if err := s.SetScoreThreshold(t.Context(), 25); err != nil {
		t.Fatal(err)
	}
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24, ScoreFilter: collectorapi.ScoreFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	assertGrade(t, result, 6, collectorapi.ScoreGradeTopQuarter, 0)

	// A favorite that an override decides has no held-out favorite score:
	// without favorite 2 the scores are 0.5, 1 and 3.
	setOverride(t, s, collectorapi.ScoreKindTag, "other:a2", collectorapi.ScoreRuleAlwaysHide)
	result, err = s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24, ScoreFilter: collectorapi.ScoreFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	if result.ScoreFilter.HeldOutFavorites != 3 {
		t.Fatalf("held-out favorites = %d", result.ScoreFilter.HeldOutFavorites)
	}
	assertGrade(t, result, 8, "", 3)

	// Removing a pin changes the scores too: favorite 4 scores 0, like gallery 8.
	pin := setPin(t, s, collectorapi.ScoreKindTag, "other:a4", 0.5)
	if err := s.RemoveScorePin(t.Context(), pin.ID); err != nil {
		t.Fatal(err)
	}
	result, err = s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24, ScoreFilter: collectorapi.ScoreFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	assertGrade(t, result, 8, "", 2)

	// Removing the override brings favorite 2 back: 0, 1, 2 and 3.
	var overrideID int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM panda_score_overrides").Scan(&overrideID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScoreOverride(t.Context(), overrideID); err != nil {
		t.Fatal(err)
	}
	assertGrade(t, listAll(t, s), 8, "", 3)

	// The author bonus lifts favorite 4 to 10.
	setFavoriteAuthor(t, db, "other:a4", 3)
	setBonus(t, s, float(10))
	assertGrade(t, listAll(t, s), 8, "", 4)

	// A preview with another default filter changes the scope of the counts,
	// but the scores keep the favorites of the scope the model was applied in.
	previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultQuery: "other:a1$"})
	setPin(t, s, collectorapi.ScoreKindTag, "other:a3", 3)
	if result := listAll(t, s); result.ScoreFilter.HeldOutFavorites != 4 {
		t.Fatalf("held-out favorites = %d", result.ScoreFilter.HeldOutFavorites)
	}

	// A refresh replaces stale scores.
	if _, err := db.Exec(t.Context(), "UPDATE panda_favorite_scores SET scores = '{}'"); err != nil {
		t.Fatal(err)
	}
	if err := s.RefreshFavoriteScores(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertGrade(t, listAll(t, s), 8, "", 4)
}

func listAll(t *testing.T, s *Service) collectorapi.CatalogResult {
	t.Helper()
	result, err := s.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24, ScoreFilter: collectorapi.ScoreFilterAll})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
