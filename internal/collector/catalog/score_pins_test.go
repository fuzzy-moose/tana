package catalog

import (
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func readPins(t *testing.T, db *pgxpool.Pool) []scorePin {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	pins, err := scorePins(t.Context(), tx)
	if err != nil {
		t.Fatal(err)
	}
	return pins
}

func pinRowCount(t *testing.T, db *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_pins").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func setPin(t *testing.T, s *Service, kind, value string, weight float64) collectorapi.ScorePin {
	t.Helper()
	pin, err := s.SetScorePin(t.Context(), kind, value, weight)
	if err != nil {
		t.Fatal(err)
	}
	return pin
}

func TestSetScorePinStoresATagByItsCatalogTag(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:Big Hat", "other:x"}}, true)
	pin := setPin(t, s, collectorapi.ScoreKindTag, " Female : big_hat ", 2.5)
	var tagID int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM catalog_tags WHERE namespace = 'female' AND value_lower = 'big hat'").Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	var storedTagID int64
	var value string
	if err := db.QueryRow(t.Context(), "SELECT tag_id, value FROM panda_score_pins WHERE kind = 'tag'").Scan(&storedTagID, &value); err != nil {
		t.Fatal(err)
	}
	if storedTagID != tagID || value != "" {
		t.Fatalf("row = %d %q, want tag %d", storedTagID, value, tagID)
	}
	want := collectorapi.ScorePin{ID: pin.ID, Kind: collectorapi.ScoreKindTag, Label: "female:Big Hat", Weight: 2.5}
	if pin.ID <= 0 || pin != want {
		t.Fatalf("pin = %+v, want %+v", pin, want)
	}
	pins := readPins(t, db)
	if len(pins) != 1 || pins[0].ScorePin != want || pins[0].TagID != tagID {
		t.Fatalf("pins = %+v", pins)
	}
}

func TestSetScorePinNormalizesUploadersAndCategories(t *testing.T) {
	db, s := openCatalog(t)
	uploader := setPin(t, s, collectorapi.ScoreKindUploader, " Some One ", 0)
	category := setPin(t, s, collectorapi.ScoreKindCategory, "Artist CG", -1e300)
	if uploader.Label != "Some One" || uploader.Weight != 0 || uploader.Kind != collectorapi.ScoreKindUploader {
		t.Fatalf("uploader = %+v", uploader)
	}
	if category.Label != "artist cg" || category.Weight != -1e300 || category.Kind != collectorapi.ScoreKindCategory {
		t.Fatalf("category = %+v", category)
	}
	pins := readPins(t, db)
	if len(pins) != 2 || pins[0].ScorePin != uploader || pins[1].ScorePin != category || pins[0].TagID != 0 || pins[1].TagID != 0 {
		t.Fatalf("pins = %+v", pins)
	}
}

func TestSetScorePinRejectsInvalidPins(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	for _, tc := range []struct {
		kind, value string
		weight      float64
		want        error
	}{
		{"tag", "female:glasses", 1, ErrTagNotFound},
		{"tag", "big hat", 1, ErrInvalidScorePin},
		{"tag", ":big hat", 1, ErrInvalidScorePin},
		{"tag", "female: ", 1, ErrInvalidScorePin},
		{"tag", "  ", 1, ErrInvalidScorePin},
		{"uploader", "   ", 1, ErrInvalidScorePin},
		{"category", "", 1, ErrInvalidScorePin},
		{"category", "comics", 1, ErrInvalidScorePin},
		{"title", "x", 1, ErrInvalidScorePin},
		{"uploader", "Some One", math.NaN(), ErrInvalidScorePin},
		{"uploader", "Some One", math.Inf(1), ErrInvalidScorePin},
		{"uploader", "Some One", math.Inf(-1), ErrInvalidScorePin},
		{"uploader", "Some One", 1.0000001e300, ErrInvalidScorePin},
		{"tag", "female:big hat", -1.0000001e300, ErrInvalidScorePin},
	} {
		if _, err := s.SetScorePin(t.Context(), tc.kind, tc.value, tc.weight); !errors.Is(err, tc.want) {
			t.Errorf("%s %q %v: err = %v, want %v", tc.kind, tc.value, tc.weight, err, tc.want)
		}
	}
	if count := pinRowCount(t, db); count != 0 {
		t.Fatalf("rows = %d", count)
	}
}

func TestSetScorePinReplacesTheWeight(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	for _, tc := range [][2]string{{"tag", "female:big hat"}, {"uploader", "Some One"}, {"category", "manga"}} {
		first := setPin(t, s, tc[0], tc[1], 1)
		second := setPin(t, s, tc[0], tc[1], -3)
		if second.ID != first.ID || second.Weight != -3 {
			t.Fatalf("%v: first = %+v, second = %+v", tc, first, second)
		}
	}
	pins := readPins(t, db)
	if len(pins) != 3 {
		t.Fatalf("pins = %+v", pins)
	}
	for _, pin := range pins {
		if pin.Weight != -3 {
			t.Fatalf("pin = %+v", pin)
		}
	}
}

func TestRemoveScorePin(t *testing.T) {
	db, s := openCatalog(t)
	kept := setPin(t, s, "uploader", "Kept", 1)
	removed := setPin(t, s, "uploader", "Removed", 1)
	if err := s.RemoveScorePin(t.Context(), removed.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScorePin(t.Context(), removed.ID); err != nil {
		t.Fatalf("missing row: %v", err)
	}
	for _, id := range []int64{0, -1} {
		if err := s.RemoveScorePin(t.Context(), id); !errors.Is(err, ErrInvalidScorePin) {
			t.Fatalf("id %d: err = %v", id, err)
		}
	}
	pins := readPins(t, db)
	if len(pins) != 1 || pins[0].ScorePin != kept {
		t.Fatalf("pins = %+v", pins)
	}
}

func TestScorePinAndOverrideCoexist(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	setOverride(t, s, "tag", "female:big hat", "always_hide")
	setPin(t, s, "tag", "female:big hat", 1)
	if overrides, pins := overrideRowCount(t, db), pinRowCount(t, db); overrides != 1 || pins != 1 {
		t.Fatalf("overrides = %d, pins = %d", overrides, pins)
	}
}
