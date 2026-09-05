package galleryinfo

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/tag"
)

func TestPartialFieldsNormalizationAndFirstValidOccurrence(t *testing.T) {
	text := "Tags: LANGUAGE:English, all ages, Multi-Work Series, language:english, :empty, artist:, artist:日本語, artist:a_b\r\n" +
		"Title:   \r\nTitle: First: title\r\nTitle: Ignored\r\n" +
		"Unknown: ignored\r\nUpload Time: invalid\r\nUpload Time: 2020-02-29 12:30\r\n" +
		"Downloaded: 2020-02-30 12:30\r\nUploaded By: Person\r\n" +
		"Uploader's Comments:\r\n\r\nAn attribution-looking line\r\n\r\n<a>raw</a>\r\n\r\nAny final wording\r\n\r\n"
	doc, diagnostics := Parse(strings.NewReader(text))
	want := []tag.Value{{Namespace: "language", Value: "english"}, {Namespace: "other", Value: "all ages"}, {Namespace: "other", Value: "multi-work series"}}
	if doc.Title != "First: title" || !reflect.DeepEqual(doc.Tags, want) || doc.UploadedBy != "Person" || !doc.Downloaded.IsZero() || doc.UploadTime.Day() != 29 {
		t.Fatalf("valid fields lost: %+v", doc)
	}
	if len(diagnostics) != 7 {
		t.Fatalf("expected seven invalid fields/tags: %v", diagnostics)
	}
	if doc.Attribution != "Any final wording" || doc.UploaderComments != "An attribution-looking line\n\n<a>raw</a>" {
		t.Fatalf("positional footer/raw comments: %+v", doc)
	}
}

func TestMissingFieldsAndPositionalFooter(t *testing.T) {
	doc, diagnostics := Parse(strings.NewReader("Tags: all ages\nDifferent downloader\n\n"))
	if len(diagnostics) != 0 || doc.Title != "" || len(doc.Tags) != 1 || doc.Attribution != "Different downloader" {
		t.Fatalf("optional fields: %+v, %v", doc, diagnostics)
	}
	doc, diagnostics = Parse(strings.NewReader("Title: retained\nTags: consumed-as-footer\n"))
	if len(diagnostics) != 0 || doc.Title != "retained" || len(doc.Tags) != 0 || doc.Attribution != "Tags: consumed-as-footer" {
		t.Fatalf("footer must be positional: %+v, %v", doc, diagnostics)
	}
	if _, diagnostics := Parse(strings.NewReader("\n")); len(diagnostics) == 0 {
		t.Fatal("empty document did not report failure")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, fmt.Errorf("read failed") }

func TestReadFailureRetainsCompleteFields(t *testing.T) {
	doc, diagnostics := Parse(io.MultiReader(strings.NewReader("Title: retained\nTags: all ages\nfooter\n"), failingReader{}))
	if doc.Title != "retained" || len(doc.Tags) != 1 || len(diagnostics) != 1 {
		t.Fatalf("partial read: %+v, %v", doc, diagnostics)
	}
}
