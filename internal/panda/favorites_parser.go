package panda

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var favoriteGalleryURL = regexp.MustCompile(`/g/(\d+)/([0-9a-f]{10})/`)
var favoriteCategoryURL = regexp.MustCompile(`(?:[?&])favcat=(\d+)(?:\D|$)`)

// ParseFavoritesPage supports the table profile used by the authenticated
// client. Unknown layouts fail closed, particularly during full reconciliation.
func ParseFavoritesPage(body []byte, category int) (FavoritesPage, error) {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return FavoritesPage{}, fmt.Errorf("%w: malformed HTML", ErrFavoritesPage)
	}
	page := FavoritesPage{}
	var table *html.Node
	var nextFound bool
	for n := range doc.Descendants() {
		if n.Type != html.ElementNode {
			continue
		}
		if n.Data == "table" && hasClass(n, "itg") {
			table = n
		}
		if attr(n, "id") == "unext" {
			nextFound = true
			page.Next = strings.TrimSpace(attr(n, "href"))
		}
		if n.Data == "option" && n.Parent != nil && attr(n.Parent, "name") == "favcat" && attr(n, "value") == strconv.Itoa(category) {
			page.CategoryName = nodeText(n)
		}
		if hasClass(n, "fp") {
			match := favoriteCategoryURL.FindStringSubmatch(attr(n, "onclick") + " " + attr(n, "href"))
			if len(match) > 0 && match[1] == strconv.Itoa(category) {
				// The category control contains a count, color swatch, then name.
				for child := range n.ChildNodes() {
					if child.Type == html.ElementNode && nodeText(child) != "" {
						page.CategoryName = nodeText(child)
					}
				}
			}
		}
		if n.Data == "select" {
			name := attr(n, "name")
			if name == "f_sort" || name == "fs" || name == "f_favsort" || name == "f_perpage" {
				for option := range n.ChildNodes() {
					if option.Data != "option" || !hasAttr(option, "selected") {
						continue
					}
					label := strings.ToLower(nodeText(option))
					if name == "f_perpage" {
						if attr(option, "value") != "100" {
							return FavoritesPage{}, fmt.Errorf("%w: expected 100 entries per page", ErrFavoritesPage)
						}
					} else if strings.Contains(label, "posted") || strings.Contains(label, "oldest") {
						return FavoritesPage{}, fmt.Errorf("%w: expected newest favorites first", ErrFavoritesPage)
					}
				}
			}
		}
	}
	if page.CategoryName == "" {
		return FavoritesPage{}, fmt.Errorf("%w: category controls missing", ErrFavoritesPage)
	}
	if table != nil {
		for row := range table.Descendants() {
			if row.Type != html.ElementNode || row.Data != "tr" {
				continue
			}
			var ref GalleryRef
			var added time.Time
			var header, data bool
			for n := range row.Descendants() {
				if n.Type != html.ElementNode {
					continue
				}
				if n.Data == "th" {
					header = true
				}
				if n.Data == "td" {
					data = true
				}
				if n.Data == "a" {
					if match := favoriteGalleryURL.FindStringSubmatch(attr(n, "href")); match != nil {
						id, err := strconv.ParseInt(match[1], 10, 64)
						if err != nil || id <= 0 || (ref.ID != 0 && ref.ID != id) {
							return FavoritesPage{}, fmt.Errorf("%w: invalid gallery reference", ErrFavoritesPage)
						}
						ref = GalleryRef{ID: id, Token: match[2]}
					}
				}
				if hasClass(n, "glfav") {
					added, err = time.Parse("2006-01-02 15:04", nodeText(n))
					if err != nil {
						return FavoritesPage{}, fmt.Errorf("%w: missing favorite timestamp", ErrFavoritesPage)
					}
				}
			}
			if header && !data {
				continue
			}
			if !data {
				continue
			}
			if ref.ID == 0 || added.IsZero() {
				return FavoritesPage{}, fmt.Errorf("%w: incomplete favorite row", ErrFavoritesPage)
			}
			if len(page.Entries) > 0 && added.After(page.Entries[len(page.Entries)-1].AddedAt) {
				return FavoritesPage{}, fmt.Errorf("%w: favorites are not newest first", ErrFavoritesPage)
			}
			page.Entries = append(page.Entries, Favorite{GalleryRef: ref, AddedAt: added})
		}
	}
	if len(page.Entries) == 0 {
		text := strings.ToLower(nodeText(doc))
		if page.Next != "" || !(strings.Contains(text, "no hits found") || strings.Contains(text, "no favorites found")) {
			return FavoritesPage{}, fmt.Errorf("%w: missing favorites or explicit empty result", ErrFavoritesPage)
		}
	}
	if len(page.Entries) > FavoritesPageSize || (page.Next != "" && len(page.Entries) != FavoritesPageSize) {
		return FavoritesPage{}, fmt.Errorf("%w: expected 100 entries on a non-final page", ErrFavoritesPage)
	}
	if len(page.Entries) == FavoritesPageSize && !nextFound {
		return FavoritesPage{}, fmt.Errorf("%w: pagination controls missing", ErrFavoritesPage)
	}
	return page, nil
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func hasClass(n *html.Node, class string) bool {
	for _, value := range strings.Fields(attr(n, "class")) {
		if value == class {
			return true
		}
	}
	return false
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	for child := range n.Descendants() {
		if child.Type == html.TextNode {
			b.WriteString(child.Data)
		}
	}
	return strings.TrimSpace(b.String())
}
