package catalog

import (
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func measure(t *testing.T, s *Service, candidateID int64) collectorapi.ScorePreview {
	t.Helper()
	return previewWith(t, s, collectorapi.ScorePreviewOptions{CandidateID: candidateID})
}

func assertSameCandidate(t *testing.T, got, want *collectorapi.ScoreCandidate) {
	t.Helper()
	if got == nil || want == nil {
		t.Fatalf("candidate %+v, want %+v", got, want)
	}
	if got.ID != want.ID || !got.CalculatedAt.Equal(want.CalculatedAt) || got.WeightCount != want.WeightCount ||
		!reflect.DeepEqual(got.Weights, want.Weights) {
		t.Fatalf("candidate %+v, want %+v", got, want)
	}
}

func TestMeasurementKeepsTheStoredCandidate(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	active := previewScore(t, s).Candidate
	if err := s.ApplyScoreModel(t.Context(), active.ID, 3); err != nil {
		t.Fatal(err)
	}
	activeRows := storedModelRows(t, db, active.ID)
	calculated := previewScore(t, s).Candidate
	if calculated.WeightCount == 0 || calculated.Weights[0].ActiveWeight == nil {
		t.Fatalf("candidate %+v", calculated)
	}
	candidateRows := storedModelRows(t, db, calculated.ID)

	assertSameCandidate(t, measure(t, s, calculated.ID).Candidate, calculated)
	if got, want := scoreModelIDs(t, db), []int64{active.ID, calculated.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	if got := storedModelRows(t, db, calculated.ID); !reflect.DeepEqual(got, candidateRows) {
		t.Fatalf("candidate rows = %v, want %v", got, candidateRows)
	}
	if got := storedModelRows(t, db, active.ID); !reflect.DeepEqual(got, activeRows) {
		t.Fatalf("active rows = %v, want %v", got, activeRows)
	}
	if row := activeModelRow(t, db); row.id != active.ID {
		t.Fatalf("active model %+v", row)
	}
}

func TestMeasurementCountsNothingAgain(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	calculated := previewScore(t, s).Candidate
	// Gallery 21 carries the weighted heldOutTags in the catalog, favorite 22
	// carries them too.
	retainRange(t, db, 21, 22, panda.Metadata{Tags: heldOutTags})
	favor(t, db, "42", 0, 22)

	measured := measure(t, s, calculated.ID)
	assertSameCandidate(t, measured.Candidate, calculated)
	if measured.Evidence.Favorites != 5 || measured.Candidate.HeldOut.Total != 5 {
		t.Fatalf("evidence %+v, held out %+v", measured.Evidence, measured.Candidate.HeldOut)
	}

	recalculated := previewScore(t, s)
	if recalculated.Candidate.ID == calculated.ID {
		t.Fatal("no new candidate")
	}
	if weight, ok := findWeight(recalculated, collectorapi.ScoreKindTag, "other:t1"); !ok || weight.Favorites != 5 || weight.Catalog != 7 {
		t.Fatalf("other:t1 = %+v", weight)
	}
}

func TestMeasurementFollowsPinsAndTheBonus(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	setFavoriteAuthor(t, db, "other:t2", 4)
	calculated := previewScore(t, s)
	setPin(t, s, collectorapi.ScoreKindTag, "other:three", 1)
	setPin(t, s, collectorapi.ScoreKindTag, "other:single", 1000)
	setBonus(t, s, float(2))

	measured := measure(t, s, calculated.Candidate.ID)
	assertSameCandidate(t, measured.Candidate, calculated.Candidate)
	if reflect.DeepEqual(measured.Candidate.HeldOut, calculated.Candidate.HeldOut) {
		t.Fatalf("held out unchanged: %+v", measured.Candidate.HeldOut)
	}
	recalculated := previewScore(t, s)
	if recalculated.Candidate.ID == measured.Candidate.ID {
		t.Fatal("no new candidate")
	}
	type measurement struct {
		HeldOut         collectorapi.ScoreHeldOut
		Suggested       int
		Recent          collectorapi.ScoreRecent
		Pins            []collectorapi.ScorePin
		AuthorBonus     *float64
		FavoriteAuthors []collectorapi.ScoreFavoriteAuthor
	}
	measurementOf := func(preview collectorapi.ScorePreview) measurement {
		return measurement{preview.Candidate.HeldOut, preview.Candidate.SuggestedThresholdTenths, preview.Recent,
			preview.Pins, preview.AuthorBonus, preview.FavoriteAuthors}
	}
	got, want := measurementOf(measured), measurementOf(recalculated)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("measurement %+v, want %+v", got, want)
	}
	if got.Pins[0].ModelWeight == nil || got.HeldOut.Scores.FirstBucket+len(got.HeldOut.Scores.Counts)-1 != scoreWindowTenths || got.FavoriteAuthors[0].Weight == nil {
		t.Fatalf("measurement %+v", got)
	}
}

func TestPreviewCalculatesForANameThatIsNoStoredCandidate(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	previous := previewScore(t, s).Candidate.ID
	for _, tc := range []struct {
		name  string
		apply bool
		id    func(previous int64) int64
	}{
		{"zero", false, func(int64) int64 { return 0 }},
		{"unknown", false, func(previous int64) int64 { return previous + 1000 }},
		{"applied", true, func(previous int64) int64 { return previous }},
	} {
		var want []int64
		if tc.apply {
			if err := s.ApplyScoreModel(t.Context(), previous, 3); err != nil {
				t.Fatal(err)
			}
			want = append(want, previous)
		}
		preview := measure(t, s, tc.id(previous))
		if preview.Candidate == nil || preview.Candidate.ID <= previous {
			t.Fatalf("%s: candidate %+v after %d", tc.name, preview.Candidate, previous)
		}
		if got := scoreModelIDs(t, db); !reflect.DeepEqual(got, append(want, preview.Candidate.ID)) {
			t.Fatalf("%s: models %v, candidate %d", tc.name, got, preview.Candidate.ID)
		}
		previous = preview.Candidate.ID
	}
}

func TestMeasurementKeepsTheCandidateWithoutFavoritesInScope(t *testing.T) {
	db, s := openCatalog(t)
	seedHeldOutFavorites(t, db)
	calculated := previewScore(t, s).Candidate
	if _, err := db.Exec(t.Context(), "DELETE FROM favorites"); err != nil {
		t.Fatal(err)
	}

	measured := measure(t, s, calculated.ID)
	assertSameCandidate(t, measured.Candidate, calculated)
	if measured.Candidate.HeldOut.Total != 0 || measured.NoModel != "" || measured.Evidence.Favorites != 0 {
		t.Fatalf("held out %+v, no model %q, evidence %+v", measured.Candidate.HeldOut, measured.NoModel, measured.Evidence)
	}
	if got, want := scoreModelIDs(t, db), []int64{calculated.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
}
