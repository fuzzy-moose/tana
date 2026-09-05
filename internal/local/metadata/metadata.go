// Package metadata defines the gallery metadata supplied by providers.
package metadata

import (
	"context"
	"io/fs"

	"github.com/fuzzy-moose/tana/internal/local/tag"
)

type Values struct {
	Title string
	Tags  []tag.Value
}

// Source exposes files in inventory encounter order and their contents. Its
// filesystem is valid only for the duration of the provider call.
type Source struct {
	Files []string
	FS    fs.FS
}

// Provider returns usable fields alongside diagnostics for unavailable or
// malformed metadata. No matching metadata is a normal, silent result.
type Provider interface {
	Read(context.Context, Source) (Values, []error)
}
