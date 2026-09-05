// Package gallery owns independently titled and ordered selections of images.
package gallery

import (
	"errors"
	"path"
	"strings"
)

var (
	ErrNotFound     = errors.New("gallery not found")
	ErrInvalidTitle = errors.New("title must not be empty")
	ErrInvalidPage  = errors.New("page must reference a supported image source file")
	ErrNoImages     = errors.New("source has no supported images")
	ErrLinkedPages  = errors.New("source-linked gallery pages may only be reordered")
)

type Gallery struct {
	ID       int64
	Title    string
	SourceID int64 // Zero for independent galleries.
}

type Page struct {
	GalleryID    int64
	Number       int64
	SourceFileID int64
}

// SupportsImage classifies filenames without decoding or reading file contents.
func SupportsImage(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".bmp", ".gif", ".jpeg", ".jpg", ".png", ".webp":
		return true
	default:
		return false
	}
}

// naturalLess compares numeric runs without integer overflow. Equal numeric
// values and case-folded names use the original path as a deterministic tie.
func naturalLess(a, b string) bool {
	x, y := strings.ToLower(a), strings.ToLower(b)
	for len(x) > 0 && len(y) > 0 {
		if digit(x[0]) && digit(y[0]) {
			i, j := 0, 0
			for i < len(x) && digit(x[i]) {
				i++
			}
			for j < len(y) && digit(y[j]) {
				j++
			}
			n, m := strings.TrimLeft(x[:i], "0"), strings.TrimLeft(y[:j], "0")
			if len(n) != len(m) {
				return len(n) < len(m)
			}
			if n != m {
				return n < m
			}
			x, y = x[i:], y[j:]
			continue
		}
		if x[0] != y[0] {
			return x[0] < y[0]
		}
		x, y = x[1:], y[1:]
	}
	if len(x) != len(y) {
		return len(x) < len(y)
	}
	return a < b
}

func digit(c byte) bool { return c >= '0' && c <= '9' }
