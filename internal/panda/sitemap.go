package panda

import (
	"bufio"
	"compress/gzip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidSitemap identifies malformed sitemap XML or invalid index entries.
var ErrInvalidSitemap = errors.New("invalid sitemap")

const maxSitemapBytes = 128 << 20

// Parse one entry at a time so even large children need only a bounded import
// batch. Local XML names support both observed 0.84 and standard sitemap XML.
func parseSitemapLocations(input io.Reader, root, entry string, visit func(string) error) error {
	buffer := bufio.NewReader(input)
	var reader io.Reader = buffer
	if magic, _ := buffer.Peek(2); len(magic) == 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		compressed, err := gzip.NewReader(buffer)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSitemap, err)
		}
		defer compressed.Close()
		reader = compressed
	}
	limited := &io.LimitedReader{R: reader, N: maxSitemapBytes + 1}
	decoder := xml.NewDecoder(limited)
	depth, roots := 0, 0
	for {
		token, err := decoder.Token()
		if limited.N == 0 {
			return fmt.Errorf("%w: document too large", ErrInvalidSitemap)
		}
		if err == io.EOF {
			if roots != 1 || depth != 0 {
				return fmt.Errorf("%w: missing root", ErrInvalidSitemap)
			}
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSitemap, err)
		}
		switch node := token.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 || node.Name.Local != root {
					return fmt.Errorf("%w: expected %s", ErrInvalidSitemap, root)
				}
			} else if depth == 1 && node.Name.Local == entry {
				var value struct {
					Locations []string `xml:"loc"`
				}
				if err := decoder.DecodeElement(&value, &node); err != nil {
					return fmt.Errorf("%w: %v", ErrInvalidSitemap, err)
				}
				location := ""
				if len(value.Locations) == 1 {
					location = strings.TrimSpace(value.Locations[0])
				}
				if err := visit(location); err != nil {
					return err
				}
				continue
			}
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(node)) != "" {
				return fmt.Errorf("%w: text outside root", ErrInvalidSitemap)
			}
		}
	}
}

// ParseSitemapIndex reads plain or gzip XML and returns distinct child URLs in
// document order. Invalid child URLs and malformed documents fail the index.
func ParseSitemapIndex(input io.Reader) ([]string, error) {
	var children []string
	seen := make(map[string]bool)
	err := parseSitemapLocations(input, "sitemapindex", "sitemap", func(location string) error {
		if !validSitemapDocumentURL(location) {
			return fmt.Errorf("%w: invalid child URL", ErrInvalidSitemap)
		}
		if !seen[location] {
			seen[location] = true
			children = append(children, location)
		}
		return nil
	})
	return children, err
}

func parseSitemapGalleryRef(location string) (GalleryRef, bool) {
	u, err := url.Parse(location)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil ||
		(u.Hostname() != "e-hentai.org" && u.Hostname() != "exhentai.org") {
		return GalleryRef{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "g" || len(parts[2]) != 10 {
		return GalleryRef{}, false
	}
	for _, c := range parts[2] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return GalleryRef{}, false
		}
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 {
		return GalleryRef{}, false
	}
	return GalleryRef{ID: id, Token: parts[2]}, true
}

// ParseSitemap streams valid gallery references from plain or gzip XML. Invalid
// gallery locations are skipped and counted. References already visited and the
// invalid count remain available when parsing fails; visit errors are returned
// unchanged. A successful parse consumes the entire document, including the
// gzip checksum. Fetching and persistence belong to the caller.
func ParseSitemap(input io.Reader, visit func(GalleryRef) error) (invalidLocations int64, err error) {
	err = parseSitemapLocations(input, "urlset", "url", func(location string) error {
		ref, valid := parseSitemapGalleryRef(location)
		if !valid {
			invalidLocations++
			return nil
		}
		return visit(ref)
	})
	return invalidLocations, err
}

func validSitemapDocumentURL(address string) bool {
	u, err := url.Parse(address)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == ""
}
