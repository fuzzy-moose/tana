package catalog

import (
	"encoding/json"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func tagWeights(t *testing.T, s *Service, tags ...string) collectorapi.ScoreTagWeights {
	t.Helper()
	weights, err := s.ScoreTagWeights(t.Context(), tags)
	if err != nil {
		t.Fatal(err)
	}
	if len(weights.Tags) != len(tags) {
		t.Fatalf("entries = %+v, want %d", weights.Tags, len(tags))
	}
	for i, tag := range tags {
		if weights.Tags[i].Tag != tag {
			t.Fatalf("entry %d = %q, want %q", i, weights.Tags[i].Tag, tag)
		}
	}
	return weights
}

// assertTagWeight checks one entry: known, its weight (nil for none) and
// its pin (nil for none).
func assertTagWeight(t *testing.T, entry collectorapi.ScoreTagWeight, known bool, weight *float64, pin *collectorapi.ScorePin) {
	t.Helper()
	if entry.Known != known {
		t.Fatalf("%s known = %v", entry.Tag, entry.Known)
	}
	if (entry.Weight == nil) != (weight == nil) || (weight != nil && *entry.Weight != *weight) {
		t.Fatalf("%s weight = %v, want %v", entry.Tag, deref(entry.Weight), deref(weight))
	}
	if (entry.Pin == nil) != (pin == nil) || (pin != nil && *entry.Pin != *pin) {
		t.Fatalf("%s pin = %+v, want %+v", entry.Tag, entry.Pin, pin)
	}
}

func deref(weight *float64) any {
	if weight == nil {
		return nil
	}
	return *weight
}

func number(weight float64) *float64 { return &weight }

func TestScoreTagWeightsWithoutAnActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	retainTagged(t, db, 1, "female:Big Hat", "artist:rare", "artist:fav")

	empty := tagWeights(t, s)
	if body, err := json.Marshal(empty); err != nil || string(body) != `{"active":false,"tags":[]}` {
		t.Fatalf("empty = %s, %v", body, err)
	}
	if nilTags, err := s.ScoreTagWeights(t.Context(), nil); err != nil || nilTags.Tags == nil {
		t.Fatalf("nil list = %+v, %v", nilTags, err)
	}

	weights := tagWeights(t, s, " Female : big_hat ", "female:unknown", "nocolon", ":hat", "female: ", "artist:rare")
	if weights.Active {
		t.Fatal("active without a model")
	}
	for i, known := range []bool{true, false, false, false, false, true} {
		assertTagWeight(t, weights.Tags[i], known, nil, nil)
	}

	pin := setPin(t, s, collectorapi.ScoreKindTag, "artist:rare", 0)
	makeFavoriteAuthor(t, db, "artist:fav")
	setBonus(t, s, number(2))
	weights = tagWeights(t, s, "artist:rare", "artist:fav", "female:big hat")
	assertTagWeight(t, weights.Tags[0], true, number(0), &collectorapi.ScorePin{ID: pin.ID, Kind: collectorapi.ScoreKindTag, Label: "artist:rare", Weight: 0})
	assertTagWeight(t, weights.Tags[1], true, number(2), nil)
	assertTagWeight(t, weights.Tags[2], true, nil, nil)

	setBonus(t, s, nil)
	assertTagWeight(t, tagWeights(t, s, "artist:fav").Tags[0], true, nil, nil)
}

func TestScoreTagWeightsUnderTheActiveModel(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	preview := applyCandidate(t, s, 0)
	retainTagged(t, db, 201, "artist:rare", "artist:fav")
	hat := weightOf(t, preview, collectorapi.ScoreKindTag, "male:hat")

	glasses := setPin(t, s, collectorapi.ScoreKindTag, "Female:Glasses", 1.5)
	artistB := setPin(t, s, collectorapi.ScoreKindTag, "artist:b", -1)
	for _, label := range []string{"artist:a", "artist:b", "artist:fav"} {
		makeFavoriteAuthor(t, db, label)
	}
	setBonus(t, s, number(5))

	weights := tagWeights(t, s, "male:hat", "artist:rare", "female:glasses", "artist:a", "artist:fav", "artist:b", "male:none")
	if !weights.Active {
		t.Fatal("not active with a model")
	}
	assertTagWeight(t, weights.Tags[0], true, &hat, nil)
	assertTagWeight(t, weights.Tags[1], true, nil, nil)
	assertTagWeight(t, weights.Tags[2], true, number(1.5), &glasses)
	assertTagWeight(t, weights.Tags[3], true, number(5), nil)
	assertTagWeight(t, weights.Tags[4], true, number(5), nil)
	assertTagWeight(t, weights.Tags[5], true, number(-1), &artistB)
	assertTagWeight(t, weights.Tags[6], false, nil, nil)

	setBonus(t, s, nil)
	artistA := weightOf(t, preview, collectorapi.ScoreKindTag, "artist:a")
	weights = tagWeights(t, s, "artist:a", "artist:fav")
	assertTagWeight(t, weights.Tags[0], true, &artistA, nil)
	assertTagWeight(t, weights.Tags[1], true, nil, nil)
}
