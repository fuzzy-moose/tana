package catalog

import (
	"errors"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestSetScorePinExpandsANamespaceShortForm(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	var tagID int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM catalog_tags WHERE namespace = 'female' AND value_lower = 'big hat'").Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"f:big hat", "F:Big_Hat"} {
		pin := setPin(t, s, collectorapi.ScoreKindTag, value, 1.5)
		if pin.Label != "female:big hat" || pin.Kind != collectorapi.ScoreKindTag || pin.Weight != 1.5 {
			t.Fatalf("%q: pin = %+v", value, pin)
		}
		pins := readPins(t, db)
		if len(pins) != 1 || pins[0].TagID != tagID || pins[0].ScorePin != pin {
			t.Fatalf("%q: pins = %+v", value, pins)
		}
	}
}

func TestSetScoreOverrideExpandsANamespaceShortForm(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	if err := s.SetScoreOverride(t.Context(), collectorapi.ScoreKindTag, "f:big hat", collectorapi.ScoreRuleAlwaysHide); err != nil {
		t.Fatal(err)
	}
	var tagID int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM catalog_tags WHERE namespace = 'female' AND value_lower = 'big hat'").Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	overrides := readOverrides(t, db)
	if len(overrides) != 1 || overrides[0].TagID != tagID || overrides[0].Label != "female:big hat" ||
		overrides[0].Rule != collectorapi.ScoreRuleAlwaysHide {
		t.Fatalf("overrides = %+v", overrides)
	}
}

func TestSavingAShortFormOfAnUnknownTagFails(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat", "f:small hat"}}, true)
	if _, err := s.SetScorePin(t.Context(), collectorapi.ScoreKindTag, "f:small hat", 1); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("pin err = %v", err)
	}
	if err := s.SetScoreOverride(t.Context(), collectorapi.ScoreKindTag, "f:small hat", collectorapi.ScoreRuleAlwaysHide); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("override err = %v", err)
	}
	if pinRowCount(t, db) != 0 || overrideRowCount(t, db) != 0 {
		t.Fatal("stored a row for an unknown tag")
	}
}

func TestScoreTagWeightsDoesNotExpandAShortForm(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	weights := tagWeights(t, s, "f:big hat", "female:big hat")
	assertTagWeight(t, weights.Tags[0], false, nil, nil)
	assertTagWeight(t, weights.Tags[1], true, nil, nil)
}
