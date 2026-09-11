package panda

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func sitemapFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/sitemap/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func gzipSitemap(t *testing.T, body []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestParseObservedSitemapShapes(t *testing.T) {
	children, err := ParseSitemapIndex(bytes.NewReader(sitemapFixture(t, "index.xml")))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(children, []string{"https://e-hentai.org/sitemap20.xml.gz", "https://e-hentai.org/sitemap21.xml.gz"}) {
		t.Fatalf("children = %v", children)
	}
	for _, name := range []string{"child.xml", "prefixed.xml"} {
		for _, gzipInput := range []bool{false, true} {
			body := sitemapFixture(t, name)
			if gzipInput {
				body = gzipSitemap(t, body)
			}
			var refs []GalleryRef
			invalid, err := ParseSitemap(bytes.NewReader(body), func(ref GalleryRef) error {
				refs = append(refs, ref)
				return nil
			})
			want := []GalleryRef{{ID: 100, Token: "012345678a"}, {ID: 101, Token: "abcdef0123"}}
			if err != nil || invalid != 0 || !reflect.DeepEqual(refs, want) {
				t.Fatalf("%s gzip=%v: refs=%v invalid=%d err=%v", name, gzipInput, refs, invalid, err)
			}
		}
	}
}

func TestSitemapRejectsBrokenDocumentsButEmitsCompleteEntries(t *testing.T) {
	for _, body := range [][]byte{sitemapFixture(t, "broken.xml"), append(sitemapFixture(t, "child.xml"), []byte("<urlset/>")...), []byte("<html/>"), gzipSitemap(t, sitemapFixture(t, "broken.xml"))} {
		var count int
		_, err := ParseSitemap(bytes.NewReader(body), func(GalleryRef) error { count++; return nil })
		if !errors.Is(err, ErrInvalidSitemap) {
			t.Fatalf("error = %v", err)
		}
		if bytes.Contains(body, []byte("/100/")) && count == 0 {
			t.Fatal("lost complete entry")
		}
	}
	brokenChecksum := gzipSitemap(t, sitemapFixture(t, "child.xml"))
	brokenChecksum[len(brokenChecksum)-8] ^= 1
	if _, err := ParseSitemap(bytes.NewReader(brokenChecksum), func(GalleryRef) error { return nil }); !errors.Is(err, ErrInvalidSitemap) {
		t.Fatalf("gzip checksum error = %v", err)
	}
}

func TestSitemapSkipsAndCountsInvalidGalleryLocations(t *testing.T) {
	locations := []string{"", "https://e-hentai.org/g/0/012345678a/", "https://e-hentai.org/g/100/no-token/", "https://elsewhere.invalid/g/100/012345678a/", "https://e-hentai.org/g/99999999999999999999999/012345678a/", "/g/100/012345678a/"}
	var body strings.Builder
	body.WriteString(`<urlset><url><lastmod>2026-01-01</lastmod></url>`)
	for _, location := range locations {
		fmt.Fprintf(&body, "<url><loc>%s</loc></url>", location)
	}
	body.WriteString("</urlset>")
	invalid, err := ParseSitemap(strings.NewReader(body.String()), func(ref GalleryRef) error {
		t.Fatalf("accepted invalid reference: %+v", ref)
		return nil
	})
	if err != nil || invalid != int64(len(locations)+1) {
		t.Fatalf("invalid=%d, err=%v", invalid, err)
	}
}

func TestSitemapPreservesInvalidCountOnFailure(t *testing.T) {
	body := `<urlset><url><loc>invalid</loc></url><url><loc>https://e-hentai.org/g/100/012345678a/</loc></url>`
	visitFailure := errors.New("import failed")
	for _, failure := range []error{nil, visitFailure} {
		var visited int
		invalid, err := ParseSitemap(strings.NewReader(body), func(GalleryRef) error { visited++; return failure })
		wantError := visitFailure
		if failure == nil {
			wantError = ErrInvalidSitemap
		}
		if invalid != 1 || visited != 1 || !errors.Is(err, wantError) {
			t.Fatalf("invalid=%d visited=%d err=%v", invalid, visited, err)
		}
	}
}

// Reader failures after the closing element must prevent successful imports.
func TestSitemapConsumesEntireInput(t *testing.T) {
	input := io.MultiReader(strings.NewReader("<urlset/>"), sitemapErrorReader{})
	if _, err := ParseSitemap(input, func(GalleryRef) error { return nil }); !errors.Is(err, ErrInvalidSitemap) {
		t.Fatalf("error = %v", err)
	}
}

type sitemapErrorReader struct{}

func (sitemapErrorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
