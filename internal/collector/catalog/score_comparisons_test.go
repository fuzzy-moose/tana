package catalog

import (
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

// activeCalculatedAt reads when the active model was calculated.
func activeCalculatedAt(t *testing.T, db *pgxpool.Pool) time.Time {
	t.Helper()
	var calculatedAt time.Time
	if err := db.QueryRow(t.Context(), "SELECT calculated_at FROM panda_score_models WHERE active").Scan(&calculatedAt); err != nil {
		t.Fatal(err)
	}
	return calculatedAt
}

// observe records that the collector first observed the gallery as a
// favorite at the time.
func observe(t *testing.T, db *pgxpool.Pool, galleryID int64, at time.Time) {
	t.Helper()
	if _, err := db.Exec(t.Context(), "INSERT INTO favorite_observations (gallery_id, observed_at) VALUES ($1, $2)", galleryID, at); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewCountsNewFavoritesSinceTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 4, panda.Metadata{Category: "Manga"})
	retain(t, db, panda.Metadata{ID: 5, Posted: 5, Category: "Doujinshi"}, true)
	favor(t, db, "42", 0, 1, 2, 3, 4, 5)
	favor(t, db, "42", 1, 3)
	if got := previewScore(t, s).Evidence.NewFavorites; got != nil {
		t.Fatalf("new favorites without an active model = %d", *got)
	}

	applyScoreModel(t, db, s, 0)
	calculated := activeCalculatedAt(t, db)
	// Gallery 2 was observed before the calculation although its favorite row
	// is newer; gallery 3 is in two collections; gallery 4 has no
	// observation; gallery 5 is outside the manga filter.
	observe(t, db, 1, calculated.Add(time.Hour))
	observe(t, db, 2, calculated.Add(-time.Hour))
	observe(t, db, 3, calculated.Add(time.Minute))
	observe(t, db, 5, calculated.Add(time.Hour))
	if _, err := db.Exec(t.Context(), "UPDATE favorites SET added_at = $1 WHERE gallery_id = 2", calculated.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	manga := previewWith(t, s, collectorapi.ScorePreviewOptions{DefaultCategories: []string{"manga"}}).Evidence.NewFavorites
	if manga == nil || *manga != 2 {
		t.Fatalf("new favorites in manga = %v, want 2", manga)
	}
	if all := previewScore(t, s).Evidence.NewFavorites; all == nil || *all != 3 {
		t.Fatalf("new favorites = %v, want 3", all)
	}
}

func TestPreviewComparesWithTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	retainRange(t, db, 1, 3, panda.Metadata{Tags: []string{"other:a"}})
	retainRange(t, db, 4, 6, panda.Metadata{Tags: []string{"other:b"}})
	retainRange(t, db, 7, 10, panda.Metadata{})
	favor(t, db, "42", 0, 1, 2, 3)
	first := previewScore(t, s)
	if first.Active != nil || first.Recent.Active != nil {
		t.Fatalf("without an active model: active %+v, recent %+v", first.Active, first.Recent.Active)
	}
	activeA, ok := findWeight(first, collectorapi.ScoreKindTag, "other:a")
	if !ok {
		t.Fatalf("candidate %+v", first.Candidate)
	}
	if err := s.ApplyScoreModel(t.Context(), first.Candidate.ID, 4); err != nil {
		t.Fatal(err)
	}

	favor(t, db, "42", 0, 4, 5, 6)
	preview := previewScore(t, s)
	if preview.Active == nil || !preview.Active.CalculatedAt.Equal(first.Candidate.CalculatedAt) || preview.Active.ThresholdTenths != 4 || preview.Recent.Active == nil {
		t.Fatalf("active %+v, recent %+v", preview.Active, preview.Recent.Active)
	}
	a, okA := findWeight(preview, collectorapi.ScoreKindTag, "other:a")
	b, okB := findWeight(preview, collectorapi.ScoreKindTag, "other:b")
	if !okA || !okB || a.ActiveWeight == nil || *a.ActiveWeight != activeA.Weight || b.ActiveWeight != nil {
		t.Fatalf("other:a %+v, other:b %+v, active weight of other:a %g", a, b, activeA.Weight)
	}
}

func TestPreviewPinsCarryTheCandidateWeight(t *testing.T) {
	db, s := openCatalog(t)
	// Favorites 1 to 3 carry 500 strong tags and other:weak, which galleries
	// 4 to 10 carry too, so it is the 501st weight. other:none is on no
	// favorite.
	strong := make([]string, scorePreviewWeights)
	for i := range strong {
		strong[i] = fmt.Sprintf("other:s%03d", i)
	}
	retainRange(t, db, 1, 3, panda.Metadata{Tags: append(strong, "other:weak")})
	retainRange(t, db, 4, 10, panda.Metadata{Tags: []string{"other:weak"}})
	retainRange(t, db, 11, 20, panda.Metadata{Tags: []string{"other:none"}})
	favor(t, db, "42", 0, 1, 2, 3)
	weak := setPin(t, s, collectorapi.ScoreKindTag, "other:weak", 1)
	none := setPin(t, s, collectorapi.ScoreKindTag, "other:none", -2)
	listed := setPin(t, s, collectorapi.ScoreKindTag, "other:s000", 3)

	preview := previewScore(t, s)
	if preview.Candidate == nil || preview.Candidate.WeightCount != scorePreviewWeights+1 {
		t.Fatalf("candidate %+v", preview.Candidate)
	}
	if _, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:weak"); ok {
		t.Fatal("other:weak is listed")
	}
	if got := []int64{preview.Pins[0].ID, preview.Pins[1].ID, preview.Pins[2].ID}; len(preview.Pins) != 3 || !reflect.DeepEqual(got, []int64{weak.ID, none.ID, listed.ID}) {
		t.Fatalf("pins %+v", preview.Pins)
	}
	wantWeak, wantListed := math.Log(13/(10+3*10/20.0)), math.Log(13/(10+3*3/20.0))
	if got := preview.Pins[0].ModelWeight; got == nil || math.Abs(*got-wantWeak) > 1e-12 {
		t.Fatalf("other:weak model weight %v, want %g", got, wantWeak)
	}
	if got := preview.Pins[1].ModelWeight; got != nil {
		t.Fatalf("other:none model weight %g", *got)
	}
	if got := preview.Pins[2].ModelWeight; got == nil || math.Abs(*got-wantListed) > 1e-12 {
		t.Fatalf("other:s000 model weight %v, want %g", got, wantListed)
	}

	if _, err := db.Exec(t.Context(), "DELETE FROM favorites"); err != nil {
		t.Fatal(err)
	}
	preview = previewScore(t, s)
	if preview.Candidate != nil || len(preview.Pins) != 3 {
		t.Fatalf("candidate %+v, pins %+v", preview.Candidate, preview.Pins)
	}
	for _, pin := range preview.Pins {
		if pin.ModelWeight != nil {
			t.Fatalf("pin without a candidate %+v", pin)
		}
	}
}

func TestPreviewFavoriteAuthorsWeighUnderTheCandidate(t *testing.T) {
	db, s := openCatalog(t)
	// artist:a is only on favorites 1 to 3, artist:c also on 4 to 10, so it
	// weighs less; artist:d is on no favorite.
	retainRange(t, db, 1, 3, panda.Metadata{Tags: []string{"artist:a", "artist:b", "artist:c"}})
	retainRange(t, db, 4, 10, panda.Metadata{Tags: []string{"artist:c"}})
	retainRange(t, db, 11, 20, panda.Metadata{Tags: []string{"artist:d"}})
	favor(t, db, "42", 0, 1, 2, 3)
	setFavoriteAuthor(t, db, "artist:a", 5)
	setFavoriteAuthor(t, db, "artist:b", 4)
	setFavoriteAuthor(t, db, "artist:c", 3)
	setFavoriteAuthor(t, db, "artist:d", 2)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:b", -1)
	weightA, weightC := math.Log(13/(10+3*3/20.0)), math.Log(13/(10+3*10/20.0))

	authors := func(bonus *float64, want ...*float64) {
		t.Helper()
		preview := previewScore(t, s)
		if !reflect.DeepEqual(preview.AuthorBonus, bonus) || preview.FavoriteAuthorCount != 4 || len(preview.FavoriteAuthors) != 4 {
			t.Fatalf("bonus %v, count %d, authors %s", preview.AuthorBonus, preview.FavoriteAuthorCount, authorsString(preview.FavoriteAuthors))
		}
		for i, label := range []string{"artist:a", "artist:b", "artist:c", "artist:d"} {
			got := preview.FavoriteAuthors[i]
			if got.Label != label || got.Favorites != 5-i || (got.Weight == nil) != (want[i] == nil) ||
				(got.Weight != nil && math.Abs(*got.Weight-*want[i]) > 1e-12) {
				t.Fatalf("authors %s, want %s with weight %v", authorsString(preview.FavoriteAuthors), label, want[i])
			}
		}
	}
	authors(nil, float(weightA), float(-1), float(weightC), nil)
	bonus := (weightA + weightC) / 2
	setBonus(t, s, float(bonus))
	authors(float(bonus), float(weightA), float(-1), float(bonus), float(bonus))
}
