package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestPreviewListsThePins(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	// Gallery 11 has no verdict, so no evidence gallery carries other:lonely.
	retain(t, db, panda.Metadata{ID: 11, Posted: 11, Tags: []string{"other:lonely"}}, true)
	preview := previewScore(t, s, false)
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"pins":[]`) {
		t.Fatalf("preview JSON lacks empty pins: %s", body)
	}

	want := []collectorapi.ScorePin{
		setPin(t, s, collectorapi.ScoreKindCategory, "Manga", 2),
		setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 1.5),
		setPin(t, s, collectorapi.ScoreKindUploader, "Bad One", -3),
		setPin(t, s, collectorapi.ScoreKindTag, "other:lonely", 1),
	}
	stored, err := json.Marshal(want[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "model_weight") {
		t.Fatalf("stored pin JSON has a model weight: %s", stored)
	}
	for i := range want[:3] {
		weight := weightOf(t, preview, want[i].Kind, want[i].Label)
		want[i].ModelWeight = &weight
	}
	preview = previewScore(t, s, false)
	if got := preview.Pins; !reflect.DeepEqual(got, want) {
		t.Fatalf("pins = %+v, want %+v", got, want)
	}
	body, err = json.Marshal(preview.Pins)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(body), `"model_weight":`); got != 3 {
		t.Fatalf("preview pins JSON has %d model weights, want 3: %s", got, body)
	}
}

func TestPreviewWithoutCandidateGivesNoPinAModelWeight(t *testing.T) {
	db, s := openCatalog(t)
	seedVerdicts(t, db, s, 1, 3, collectorapi.VerdictInteresting, "Bad One", "Manga", "male:hat")
	setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 1)
	setPin(t, s, collectorapi.ScoreKindUploader, "Bad One", 1)
	preview := previewScore(t, s, false)
	if preview.Candidate != nil {
		t.Fatalf("preview has a candidate: %+v", preview.Candidate)
	}
	for _, pin := range preview.Pins {
		if pin.ModelWeight != nil {
			t.Fatalf("pin %+v has model weight %g", pin, *pin.ModelWeight)
		}
	}
}

func TestPinModelWeightComesFromAllWeights(t *testing.T) {
	db, s := openCatalog(t)
	fillers := make([]string, 0, scorePreviewWeights+10)
	for i := range scorePreviewWeights + 10 {
		fillers = append(fillers, fmt.Sprintf("other:filler %d", i))
	}
	// The fillers weigh like the artist tags of seedScoreEvidence; other:weak,
	// on one "Not for me" and two "Interesting" galleries, weighs less.
	seedVerdicts(t, db, s, 1, 3, collectorapi.VerdictNotForMe, "", "", append([]string{"female:glasses"}, fillers...)...)
	seedVerdicts(t, db, s, 4, 1, collectorapi.VerdictNotForMe, "", "", "female:glasses", "other:weak")
	seedVerdicts(t, db, s, 5, 1, collectorapi.VerdictNotForMe, "", "", "female:glasses")
	seedVerdicts(t, db, s, 6, 2, collectorapi.VerdictInteresting, "", "", "male:hat", "other:weak")
	seedVerdicts(t, db, s, 8, 3, collectorapi.VerdictInteresting, "", "", "male:hat")
	setPin(t, s, collectorapi.ScoreKindTag, "other:weak", 5)

	preview := previewScore(t, s, false)
	if got := len(preview.Candidate.Weights); got != scorePreviewWeights {
		t.Fatalf("candidate lists %d weights, want %d", got, scorePreviewWeights)
	}
	if _, ok := findWeight(preview, collectorapi.ScoreKindTag, "other:weak"); ok {
		t.Fatal("candidate lists other:weak")
	}
	weight := preview.Pins[0].ModelWeight
	if weight == nil {
		t.Fatalf("pin %+v has no model weight", preview.Pins[0])
	}
	if weakest := preview.Candidate.Weights[scorePreviewWeights-1].Weight; *weight <= 0 || *weight >= math.Abs(weakest) {
		t.Fatalf("model weight = %g, want between 0 and %g", *weight, math.Abs(weakest))
	}
}

func TestHeldOutScoresCountPins(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	before := previewScore(t, s, false)
	glasses := weightOf(t, before, collectorapi.ScoreKindTag, "female:glasses")

	// Galleries 4 and 5 carry only female:glasses, 1 to 3 also artist:a and
	// the uploader, 6 to 8 male:hat and manga. No fold weighs artist:a, the
	// uploader or manga.
	setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", 0)
	setPin(t, s, collectorapi.ScoreKindTag, "artist:a", -2)
	setPin(t, s, collectorapi.ScoreKindUploader, "Bad One", -1)
	setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 100)
	setPin(t, s, collectorapi.ScoreKindCategory, "Manga", 1)
	after := previewScore(t, s, false)
	heldOut := after.Candidate.HeldOut
	if got, want := histogramCounts(t, &heldOut.NotForMe.Scores), map[int]int{-30: 3, 0: 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("not for me counts = %v, want %v", got, want)
	}
	if got, want := histogramCounts(t, &heldOut.Interesting.Scores), map[int]int{1000: 2, 1010: 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("interesting counts = %v, want %v", got, want)
	}
	if got := after.Candidate.SuggestedThresholdTenths; got < 1000 || got <= before.Candidate.SuggestedThresholdTenths {
		t.Fatalf("suggested threshold = %d, before %d", got, before.Candidate.SuggestedThresholdTenths)
	}
	if got := weightOf(t, after, collectorapi.ScoreKindTag, "female:glasses"); got != glasses {
		t.Fatalf("female:glasses weight = %g, want %g", got, glasses)
	}
	if !reflect.DeepEqual(after.Candidate.Weights, before.Candidate.Weights) {
		t.Fatalf("weights with pins %+v, without %+v", after.Candidate.Weights, before.Candidate.Weights)
	}
}

func TestPreviewRecentCountsPins(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	pin := setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 100)
	pinned := map[int]int{-7: 1, 1000: 1, 0: 1, -36: 1}
	if got := histogramCounts(t, previewScore(t, s, false).Recent.Candidate); !reflect.DeepEqual(got, pinned) {
		t.Fatalf("candidate counts = %v, want %v", got, pinned)
	}
	applyCandidate(t, s, 0)
	if got := histogramCounts(t, previewScore(t, s, false).Recent.Active); !reflect.DeepEqual(got, pinned) {
		t.Fatalf("active counts = %v, want %v", got, pinned)
	}

	if err := s.RemoveScorePin(t.Context(), pin.ID); err != nil {
		t.Fatal(err)
	}
	preview := previewScore(t, s, false)
	calculated := map[int]int{-7: 1, 6: 1, 0: 1, -36: 1}
	for name, histogram := range map[string]*collectorapi.ScoreHistogram{"candidate": preview.Recent.Candidate, "active": preview.Recent.Active} {
		if got := histogramCounts(t, histogram); !reflect.DeepEqual(got, calculated) {
			t.Fatalf("%s counts = %v, want %v", name, got, calculated)
		}
	}
}

func TestPreviewWorksWithPinsOfAnySize(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
	seedScoredGalleries(t, db)
	setPin(t, s, collectorapi.ScoreKindTag, "female:glasses", -1e6)
	setPin(t, s, collectorapi.ScoreKindTag, "male:hat", 1e12)

	preview := previewScore(t, s, false)
	heldOut := preview.Candidate.HeldOut
	if got, want := histogramCounts(t, &heldOut.NotForMe.Scores), map[int]int{-scoreWindowTenths: 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("not for me counts = %v, want %v", got, want)
	}
	if got, want := histogramCounts(t, &heldOut.Interesting.Scores), map[int]int{scoreWindowTenths: 5}; !reflect.DeepEqual(got, want) {
		t.Fatalf("interesting counts = %v, want %v", got, want)
	}
	recent := map[int]int{-scoreWindowTenths: 2, 0: 1, scoreWindowTenths: 1}
	if got := histogramCounts(t, preview.Recent.Candidate); !reflect.DeepEqual(got, recent) {
		t.Fatalf("candidate counts = %v, want %v", got, recent)
	}
	body, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) >= 1<<20 {
		t.Fatalf("preview JSON has %d bytes", len(body))
	}
	if err := s.ApplyScoreModel(t.Context(), preview.Candidate.ID, preview.Candidate.SuggestedThresholdTenths); err != nil {
		t.Fatalf("apply the suggested threshold %d: %v", preview.Candidate.SuggestedThresholdTenths, err)
	}

	preview = previewScore(t, s, false)
	if got := histogramCounts(t, preview.Recent.Active); !reflect.DeepEqual(got, recent) {
		t.Fatalf("active counts = %v, want %v", got, recent)
	}
	heldOut = preview.Candidate.HeldOut
	for _, histogram := range []*collectorapi.ScoreHistogram{preview.Recent.Active, preview.Recent.Candidate, &heldOut.Interesting.Scores, &heldOut.NotForMe.Scores, &heldOut.Neutral.Scores} {
		if len(histogram.Counts) > 2*scoreWindowTenths+1 {
			t.Fatalf("histogram has %d counts", len(histogram.Counts))
		}
	}
}
