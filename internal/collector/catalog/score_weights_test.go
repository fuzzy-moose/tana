package catalog

import (
	"math"
	"testing"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestScoreWeightNeedsSupportAndBothSides(t *testing.T) {
	balanced := scoreCounts{interesting: 20, notForMe: 20, neutral: 100}
	for _, tc := range []struct {
		name  string
		total scoreCounts
		value scoreCounts
		ok    bool
	}{
		{"enough support", balanced, scoreCounts{interesting: 1, notForMe: 1, neutral: 1}, true},
		{"too little support", balanced, scoreCounts{interesting: 1, notForMe: 1}, false},
		{"no not for me", scoreCounts{interesting: 20, neutral: 100}, scoreCounts{interesting: 5}, false},
		{"no acceptable", scoreCounts{notForMe: 20}, scoreCounts{notForMe: 5}, false},
		{"neutral only acceptable", scoreCounts{notForMe: 20, neutral: 100}, scoreCounts{notForMe: 5}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := scoreWeight(tc.total, tc.value); ok != tc.ok {
				t.Fatalf("weight ok = %v, want %v", ok, tc.ok)
			}
		})
	}
	if _, ok := scoreWeights(scoreCounts{interesting: 20}, map[scoreValue]scoreCounts{{kind: collectorapi.ScoreKindTag, tagID: 1}: {interesting: 5}}); ok {
		t.Fatal("model without not for me evidence")
	}
	weights, ok := scoreWeights(balanced, map[scoreValue]scoreCounts{
		{kind: collectorapi.ScoreKindTag, tagID: 1}:        {notForMe: 5},
		{kind: collectorapi.ScoreKindTag, tagID: 2}:        {notForMe: 2},
		{kind: collectorapi.ScoreKindUploader, value: "u"}: {interesting: 3},
	})
	if !ok || len(weights) != 2 {
		t.Fatalf("weights = %v, %v", weights, ok)
	}
}

func TestScoreWeightDirection(t *testing.T) {
	total := scoreCounts{interesting: 40, notForMe: 120, neutral: 500}
	weight := func(value scoreCounts) float64 {
		t.Helper()
		w, ok := scoreWeight(total, value)
		if !ok {
			t.Fatalf("no weight for %+v", value)
		}
		return w
	}
	if w := weight(scoreCounts{notForMe: 10}); w >= 0 {
		t.Fatalf("not for me only = %v", w)
	}
	if w := weight(scoreCounts{interesting: 10}); w <= 0 {
		t.Fatalf("interesting only = %v", w)
	}
	for _, scale := range []int{1, 10} {
		if w := weight(scoreCounts{interesting: 4 * scale, notForMe: 12 * scale, neutral: 50 * scale}); math.Abs(w) > 1e-9 {
			t.Fatalf("proportional value x%d = %v", scale, w)
		}
	}
}

func TestScoreWeightGrowsWithEvidence(t *testing.T) {
	total := scoreCounts{interesting: 50, notForMe: 50}
	weight := func(total, value scoreCounts) float64 {
		t.Helper()
		w, _ := scoreWeight(total, value)
		return w
	}
	for _, tc := range []struct{ fewer, more scoreCounts }{
		{scoreCounts{interesting: 1, notForMe: 4}, scoreCounts{interesting: 4, notForMe: 16}},
		{scoreCounts{interesting: 4, notForMe: 1}, scoreCounts{interesting: 16, notForMe: 4}},
	} {
		if fewer, more := weight(total, tc.fewer), weight(total, tc.more); math.Abs(more) <= math.Abs(fewer) {
			t.Fatalf("%+v = %v, %+v = %v", tc.fewer, fewer, tc.more, more)
		}
	}
	large := scoreCounts{interesting: 1000, notForMe: 1000, neutral: 1000}
	base := weight(large, scoreCounts{notForMe: 10})
	neutral := weight(large, scoreCounts{notForMe: 10, neutral: 100})
	interesting := weight(large, scoreCounts{notForMe: 10, interesting: 100})
	if math.Abs(neutral-base) >= math.Abs(interesting-base) {
		t.Fatalf("base %v, +100 neutral %v, +100 interesting %v", base, neutral, interesting)
	}
}

func TestTallyScoreCountsSkipsEmptyUploaderAndCategory(t *testing.T) {
	total, values := tallyScoreCounts([]evidenceGallery{
		{galleryID: 1, class: scoreNotForMe, uploader: "a", category: "manga", tagIDs: []int64{7}},
		{galleryID: 2, class: scoreNeutral, tagIDs: []int64{7}},
	})
	if total != (scoreCounts{notForMe: 1, neutral: 1}) || len(values) != 3 {
		t.Fatalf("total %+v, values %+v", total, values)
	}
	if got := values[scoreValue{kind: collectorapi.ScoreKindTag, tagID: 7}]; got != (scoreCounts{notForMe: 1, neutral: 1}) {
		t.Fatalf("tag counts = %+v", got)
	}
}
