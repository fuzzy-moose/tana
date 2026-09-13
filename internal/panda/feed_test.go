package panda_test

import (
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func documentedFeedEntry(t *testing.T) string {
	t.Helper()
	doc, err := os.ReadFile("../../docs/panda-feed-entry.md")
	if err != nil {
		t.Fatal(err)
	}
	_, block, ok := strings.Cut(string(doc), "```xml\n")
	if !ok {
		t.Fatal("documented feed entry has no XML example")
	}
	entry, _, ok := strings.Cut(block, "\n```")
	if !ok {
		t.Fatal("documented feed entry has an unclosed XML example")
	}
	return entry
}

func atomFeed(entries string) string {
	return `<feed xmlns="http://www.w3.org/2005/Atom">` + entries + `</feed>`
}

func TestParseFeed(t *testing.T) {
	entry := documentedFeedEntry(t)
	input := atomFeed(entry + `<entry><link href="https://example.test/g/42/next/" /></entry>` + entry)
	got, err := panda.ParseFeed(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	preview := panda.FeedEntry{
		GalleryRef:   panda.GalleryRef{ID: 1234567, Token: "0123456789"},
		Title:        "[Example Circle] Tea & Sketches [English]",
		ThumbnailURL: "https://thumbs.example.test/w/01/234/56789-example.webp",
	}
	want := []panda.FeedEntry{preview, {GalleryRef: panda.GalleryRef{ID: 42, Token: "next"}}, preview}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
}

func TestParseFeedLinkAndPreviewExtraction(t *testing.T) {
	input := `<a:feed xmlns:a="http://www.w3.org/2005/Atom" xmlns:x="http://www.w3.org/1999/xhtml">
		<a:entry>
			<a:title>  Original title  </a:title>
			<a:link rel="self" href="https://example.test/entry" />
			<a:link rel="alternate" href="https://example.test/g/7/opaque-token/?preview=1&amp;page=2" />
			<a:updated>unused timestamp</a:updated>
			<a:content type="xhtml"><x:div>
				<x:img src="thumb.webp?width=200&amp;height=300" />
				<x:img src="another.webp" />
			</x:div></a:content>
		</a:entry>
	</a:feed>`
	got, err := panda.ParseFeed(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	want := []panda.FeedEntry{{
		GalleryRef:   panda.GalleryRef{ID: 7, Token: "opaque-token"},
		Title:        "  Original title  ",
		ThumbnailURL: "thumb.webp?width=200&height=300",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %+v, want %+v", got, want)
	}
}

func TestParseFeedEmpty(t *testing.T) {
	got, err := panda.ParseFeed(strings.NewReader(atomFeed("")))
	if err != nil || len(got) != 0 {
		t.Fatalf("empty feed = %+v, %v", got, err)
	}
}

func TestParseFeedIgnoresIllegalControlCharacters(t *testing.T) {
	controls := "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x0b\x0c\x0e\x0f" +
		"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f"
	input := atomFeed("<entry><title>Before"+controls+"After\t\n日本語</title><link href=\"https://example.test/g/7/token/\" /></entry>") + controls
	want := []panda.FeedEntry{{GalleryRef: panda.GalleryRef{ID: 7, Token: "token"}, Title: "BeforeAfter\t\n日本語"}}
	for _, tc := range []struct {
		name   string
		reader io.Reader
	}{
		{"buffered", strings.NewReader(input)},
		{"one byte", iotest.OneByteReader(strings.NewReader(input))},
		{"data with EOF", iotest.DataErrReader(strings.NewReader(input))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := panda.ParseFeed(tc.reader)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("entries = %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseFeedEntryFailuresReturnNoEntries(t *testing.T) {
	valid := documentedFeedEntry(t)
	for _, tc := range []struct {
		name  string
		entry string
	}{
		{"missing link", `<entry />`},
		{"invalid URL", `<entry><link href="https://example.test/g/%zz/token/" /></entry>`},
		{"missing token", `<entry><link href="https://example.test/g/7/" /></entry>`},
		{"invalid ID", `<entry><link href="https://example.test/g/invalid/token/" /></entry>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := panda.ParseFeed(strings.NewReader(atomFeed(valid + tc.entry)))
			if got != nil || err == nil || !strings.Contains(err.Error(), "entry 2") {
				t.Fatalf("expected failure at entry 2 with no results, got %+v, %v", got, err)
			}
		})
	}
}

func TestParseFeedXMLFailuresReturnNoEntries(t *testing.T) {
	entry := documentedFeedEntry(t)
	for _, input := range []string{
		atomFeed(entry + `<entry>`),
		atomFeed(entry) + `<`,
	} {
		got, err := panda.ParseFeed(strings.NewReader(input))
		if got != nil || err == nil {
			t.Fatalf("expected XML failure with no results, got %+v, %v", got, err)
		}
	}
}

func TestParseFeedReaderFailureReturnsNoEntries(t *testing.T) {
	wantErr := errors.New("feed read failed")
	r := io.MultiReader(strings.NewReader(atomFeed(documentedFeedEntry(t))), feedErrorReader{wantErr})
	got, err := panda.ParseFeed(r)
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected reader failure with no results, got %+v, %v", got, err)
	}
}

type feedErrorReader struct{ err error }

func (r feedErrorReader) Read([]byte) (int, error) { return 0, r.err }
