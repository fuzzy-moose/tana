package catalog

import (
	"math"
	"testing"
)

func TestScoreWeight(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		favorites, favoritesInScope, catalog, inScope int
		want                                          float64
		ok                                            bool
	}{
		{"more frequent in the favorites", 3, 4, 5, 20, math.Log(13.0 / 11), true},
		{"as frequent as in the catalog", 3, 4, 15, 20, 0, false},
		{"less frequent than in the catalog", 3, 4, 18, 20, 0, false},
		{"too few favorites", 2, 2, 2, 1000, 0, false},
		{"rare artist", 3, 1000, 5, 1000000, math.Log(13 / 10.005), true},
	} {
		got, ok := scoreWeight(tc.favorites, tc.favoritesInScope, tc.catalog, tc.inScope)
		if ok != tc.ok || math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("%s: %g, %v; want %g, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}
