package catalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestPreviewListsThePins(t *testing.T) {
	db, s := openCatalog(t)
	seedScoreEvidence(t, db, s)
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
	}
	if got := previewScore(t, s, false).Pins; !reflect.DeepEqual(got, want) {
		t.Fatalf("pins = %+v, want %+v", got, want)
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
