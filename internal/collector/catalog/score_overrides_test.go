package catalog

import (
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fuzzy-moose/tana/internal/collectorapi"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func overrideRowCount(t *testing.T, db *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := db.QueryRow(t.Context(), "SELECT count(*) FROM panda_score_overrides").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func readOverrides(t *testing.T, db *pgxpool.Pool) []scoreOverride {
	t.Helper()
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	overrides, err := scoreOverrides(t.Context(), tx)
	if err != nil {
		t.Fatal(err)
	}
	return overrides
}

func TestSetScoreOverrideStoresATagByItsCatalogTag(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat", "other:x"}}, true)
	if err := s.SetScoreOverride(t.Context(), collectorapi.ScoreKindTag, " Female : Big_Hat ", collectorapi.ScoreRuleAlwaysHide); err != nil {
		t.Fatal(err)
	}
	var tagID int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM catalog_tags WHERE namespace = 'female' AND value_lower = 'big hat'").Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	var storedTagID int64
	var value string
	if err := db.QueryRow(t.Context(), "SELECT tag_id, value FROM panda_score_overrides WHERE kind = 'tag'").Scan(&storedTagID, &value); err != nil {
		t.Fatal(err)
	}
	if storedTagID != tagID || value != "" {
		t.Fatalf("row = %d %q, want tag %d", storedTagID, value, tagID)
	}
	overrides := readOverrides(t, db)
	if len(overrides) != 1 || overrides[0].Kind != collectorapi.ScoreKindTag || overrides[0].Rule != collectorapi.ScoreRuleAlwaysHide ||
		overrides[0].Label != "female:big hat" || overrides[0].TagID != tagID || overrides[0].ID <= 0 {
		t.Fatalf("overrides = %+v", overrides)
	}
}

func TestSetScoreOverrideRejectsInvalidValues(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	for _, tc := range []struct {
		kind, value, rule string
		want              error
	}{
		{"tag", "female:glasses", "always_hide", ErrTagNotFound},
		{"tag", "big hat", "always_hide", ErrInvalidScoreOverride},
		{"tag", ":big hat", "always_hide", ErrInvalidScoreOverride},
		{"tag", "female:", "always_hide", ErrInvalidScoreOverride},
		{"tag", "female: ", "always_hide", ErrInvalidScoreOverride},
		{"tag", "  ", "always_hide", ErrInvalidScoreOverride},
		{"uploader", "", "never_hide", ErrInvalidScoreOverride},
		{"uploader", "   ", "never_hide", ErrInvalidScoreOverride},
		{"category", "", "never_hide", ErrInvalidScoreOverride},
		{"category", "comics", "never_hide", ErrInvalidScoreOverride},
		{"title", "x", "never_hide", ErrInvalidScoreOverride},
		{"uploader", "Some One", "maybe_hide", ErrInvalidScoreOverride},
		{"tag", "female:big hat", "", ErrInvalidScoreOverride},
	} {
		if err := s.SetScoreOverride(t.Context(), tc.kind, tc.value, tc.rule); !errors.Is(err, tc.want) {
			t.Errorf("%s %q %s: err = %v, want %v", tc.kind, tc.value, tc.rule, err, tc.want)
		}
	}
	if count := overrideRowCount(t, db); count != 0 {
		t.Fatalf("rows = %d", count)
	}
}

func TestSetScoreOverrideNormalizesUploadersAndCategories(t *testing.T) {
	db, s := openCatalog(t)
	if err := s.SetScoreOverride(t.Context(), "uploader", " Some One ", "never_hide"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetScoreOverride(t.Context(), "category", "Artist CG", "always_hide"); err != nil {
		t.Fatal(err)
	}
	overrides := readOverrides(t, db)
	got := make([]string, 0, len(overrides))
	for _, override := range overrides {
		if override.TagID != 0 {
			t.Fatalf("override %+v has a tag", override)
		}
		got = append(got, override.Kind+"|"+override.Label+"|"+override.Rule)
	}
	slices.Sort(got)
	if want := []string{"category|artist cg|always_hide", "uploader|Some One|never_hide"}; !slices.Equal(got, want) {
		t.Fatalf("overrides = %v, want %v", got, want)
	}
}

func TestSetScoreOverrideReplacesTheRule(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	for _, tc := range [][2]string{{"tag", "female:big hat"}, {"uploader", "Some One"}, {"category", "manga"}} {
		if err := s.SetScoreOverride(t.Context(), tc[0], tc[1], "always_hide"); err != nil {
			t.Fatal(err)
		}
		if err := s.SetScoreOverride(t.Context(), tc[0], tc[1], "never_hide"); err != nil {
			t.Fatal(err)
		}
	}
	overrides := readOverrides(t, db)
	if len(overrides) != 3 {
		t.Fatalf("overrides = %+v", overrides)
	}
	for _, override := range overrides {
		if override.Rule != "never_hide" {
			t.Fatalf("override = %+v", override)
		}
	}
}

func TestRemoveScoreOverride(t *testing.T) {
	db, s := openCatalog(t)
	for _, uploader := range []string{"Kept", "Removed"} {
		if err := s.SetScoreOverride(t.Context(), "uploader", uploader, "always_hide"); err != nil {
			t.Fatal(err)
		}
	}
	var id int64
	if err := db.QueryRow(t.Context(), "SELECT id FROM panda_score_overrides WHERE value = 'Removed'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScoreOverride(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveScoreOverride(t.Context(), id); err != nil {
		t.Fatalf("missing row: %v", err)
	}
	for _, invalid := range []int64{0, -1} {
		if err := s.RemoveScoreOverride(t.Context(), invalid); !errors.Is(err, ErrInvalidScoreOverride) {
			t.Fatalf("remove %d: %v", invalid, err)
		}
	}
	if overrides := readOverrides(t, db); len(overrides) != 1 || overrides[0].Label != "Kept" {
		t.Fatalf("overrides = %+v", overrides)
	}
}

func TestScoreOverrideSurvivesReprojection(t *testing.T) {
	db, s := openCatalog(t)
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"female:big hat"}}, true)
	retain(t, db, panda.Metadata{ID: 2, Posted: 2, Tags: []string{"female:big hat", "other:y"}}, true)
	if err := s.SetScoreOverride(t.Context(), "tag", "female:big hat", "never_hide"); err != nil {
		t.Fatal(err)
	}
	retain(t, db, panda.Metadata{ID: 1, Posted: 1, Tags: []string{"other:x"}}, true)
	retain(t, db, panda.Metadata{ID: 2, Posted: 2, Tags: []string{"other:y"}}, true)
	if overrides := readOverrides(t, db); len(overrides) != 1 || overrides[0].Label != "female:big hat" || overrides[0].Rule != "never_hide" {
		t.Fatalf("overrides = %+v", overrides)
	}
}
