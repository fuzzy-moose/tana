package panda

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

type FeedEntry struct {
	GalleryRef   GalleryRef
	Title        string
	ThumbnailURL string
}

type feedEntry struct {
	Title string `xml:"http://www.w3.org/2005/Atom title"`
	Links []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"http://www.w3.org/2005/Atom link"`
	Content struct {
		Div struct {
			Images []struct {
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
	decoder := xml.NewDecoder(r)
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
		parsed := FeedEntry{GalleryRef: ref, Title: entry.Title}
		if len(entry.Content.Div.Images) > 0 {
			parsed.ThumbnailURL = entry.Content.Div.Images[0].Src
		}
		entries = append(entries, parsed)
	}
	return entries, nil
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
