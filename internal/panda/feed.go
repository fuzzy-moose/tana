package panda

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FeedEntry struct {
	GalleryRef   GalleryRef
	Title        string
	ThumbnailURL string
	PostedAt     *time.Time
	Uploader     string
	Description  string
	Tags         []string
}

type feedEntry struct {
	Title   string `xml:"http://www.w3.org/2005/Atom title"`
	Updated string `xml:"http://www.w3.org/2005/Atom updated"`
	Summary string `xml:"http://www.w3.org/2005/Atom summary"`
	Author  struct {
		Name string `xml:"http://www.w3.org/2005/Atom name"`
	} `xml:"http://www.w3.org/2005/Atom author"`
	Links []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"http://www.w3.org/2005/Atom link"`
	Content struct {
		Div struct {
			Paragraphs []feedText `xml:"http://www.w3.org/1999/xhtml p"`
			Images     []struct {
				Src string `xml:"src,attr"`
			} `xml:"http://www.w3.org/1999/xhtml img"`
		} `xml:"http://www.w3.org/1999/xhtml div"`
	} `xml:"http://www.w3.org/2005/Atom content"`
}

// ParseFeed extracts gallery references and previews in feed order, retaining
// duplicates. Missing titles and thumbnails remain empty. Any parsing error
// returns no entries; callers own fetching, persistence, and completion tracking.
func ParseFeed(r io.Reader) ([]FeedEntry, error) {
	var feed struct {
		XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
		Entries []feedEntry `xml:"http://www.w3.org/2005/Atom entry"`
	}
	decoder := xml.NewDecoder(feedXMLReader{r})
	if err := decoder.Decode(&feed); err != nil {
		return nil, fmt.Errorf("panda: parse feed: %w", err)
	}
	// Consume the entire input so trailing XML or reader errors cannot be hidden.
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("panda: parse feed: %w", err)
		}
	}

	entries := make([]FeedEntry, 0, len(feed.Entries))
	for i, entry := range feed.Entries {
		var href string
		for _, link := range entry.Links {
			if link.Rel == "alternate" || link.Rel == "" {
				href = link.Href
				break
			}
		}
		ref, err := parseFeedGalleryRef(href)
		if err != nil {
			return nil, fmt.Errorf("panda: feed entry %d: %w", i+1, err)
		}
		parsed := FeedEntry{GalleryRef: ref, Title: entry.Title, Uploader: entry.Author.Name, Description: entry.Summary}
		// Panda uses Atom updated as the gallery's upload/publication time.
		if posted, err := time.Parse(time.RFC3339, strings.TrimSpace(entry.Updated)); err == nil {
			posted = posted.UTC()
			parsed.PostedAt = &posted
		}
		if len(entry.Content.Div.Images) > 0 {
			parsed.ThumbnailURL = entry.Content.Div.Images[0].Src
		}
		for _, paragraph := range entry.Content.Div.Paragraphs {
			for _, line := range strings.Split(string(paragraph), "\n") {
				line = strings.TrimSpace(line)
				if tags, found := strings.CutPrefix(line, "Tags:"); found {
					for _, tag := range strings.Split(tags, ",") {
						if tag = strings.TrimSpace(tag); tag != "" {
							parsed.Tags = append(parsed.Tags, tag)
						}
					}
				}
				if description, found := strings.CutPrefix(line, "Description:"); found && parsed.Description == "" {
					parsed.Description = strings.TrimSpace(description)
				}
			}
		}
		entries = append(entries, parsed)
	}
	return entries, nil
}

// Keep XHTML line breaks so description text cannot become part of a tag.
type feedText string

func (text *feedText) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	var body strings.Builder
	depth := 1
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.CharData:
			body.Write(token)
		case xml.StartElement:
			depth++
			if token.Name.Local == "br" {
				body.WriteByte('\n')
			}
		case xml.EndElement:
			depth--
		}
	}
	*text = feedText(body.String())
	return nil
}

// Upstream feed text can contain stray control bytes forbidden by XML 1.0.
// Filter those bytes while parsing; the captured raw feed remains untouched.
type feedXMLReader struct{ io.Reader }

func (r feedXMLReader) Read(p []byte) (int, error) {
	for {
		n, err := r.Reader.Read(p)
		kept := 0
		for _, b := range p[:n] {
			if b >= 0x20 || b == '\t' || b == '\n' || b == '\r' {
				p[kept] = b
				kept++
			}
		}
		if kept > 0 || err != nil || n == 0 {
			return kept, err
		}
	}
}

func parseFeedGalleryRef(href string) (GalleryRef, error) {
	link, err := url.Parse(href)
	if err != nil {
		return GalleryRef{}, fmt.Errorf("parse gallery link: %w", err)
	}
	parts := strings.Split(strings.Trim(link.Path, "/"), "/")
	if len(parts) != 3 || parts[0] != "g" || parts[2] == "" {
		return GalleryRef{}, fmt.Errorf("parse gallery link: expected /g/{id}/{token}/")
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return GalleryRef{}, fmt.Errorf("parse gallery ID: %w", err)
	}
	return GalleryRef{ID: id, Token: parts[2]}, nil
}
