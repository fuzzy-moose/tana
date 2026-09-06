package enrichment

import (
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/source"
	"github.com/fuzzy-moose/tana/internal/local/tag"
	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestCandidateUsesOnlyTrailingSourceID(t *testing.T) {
	for _, tt := range []struct {
		name string
		kind source.Kind
		want int64
	}{
		{"nested/Some random name [396226].zip", source.Archive, 396226},
		{"Name [396226].CBZ", source.Archive, 396226},
		{"Name [396226]", source.Directory, 396226},
		{"Name.zip [396226]", source.Directory, 396226},
		{"Name [10] [396226].zip", source.Archive, 396226},
		{"Name [396226].zip", source.Directory, 0},
		{"Parent [396226]/Name.zip", source.Archive, 0},
		{"Name [396226] extras.zip", source.Archive, 0},
		{"Name [396226].extra.zip", source.Archive, 0},
		{"Name [0].zip", source.Archive, 0},
		{"Name [-123].zip", source.Archive, 0},
		{"Name [999999999999999999999999999999].zip", source.Archive, 0},
	} {
		if got := candidateID(tt.name, tt.kind); got != tt.want {
			t.Errorf("%s (%s): got %d, want %d", tt.name, tt.kind, got, tt.want)
		}
	}
}

func TestMetadataMapping(t *testing.T) {
	values, diagnostics := metadataValues(panda.Metadata{Title: " API title ", TitleJapanese: "原題", Tags: []string{
		" Language:ENGLISH ", "language:english", "All Ages", "bad_tag", ":empty", "newnamespace:valid",
	}})
	want := []tag.Value{{Namespace: "language", Value: "english"}, {Namespace: "other", Value: "all ages"}, {Namespace: "newnamespace", Value: "valid"}}
	if values.Title != "API title" || !reflect.DeepEqual(values.Tags, want) || len(diagnostics) != 2 {
		t.Fatalf("mapping: %+v, %v", values, diagnostics)
	}
	values, _ = metadataValues(panda.Metadata{Title: " ", TitleJapanese: " 原題 "})
	if values.Title != "原題" {
		t.Fatalf("Japanese fallback: %q", values.Title)
	}
	values, _ = metadataValues(panda.Metadata{})
	if values.Title != "" || len(values.Tags) != 0 {
		t.Fatalf("empty metadata: %+v", values)
	}
}
