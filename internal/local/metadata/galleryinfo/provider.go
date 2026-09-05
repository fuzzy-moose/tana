package galleryinfo

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/fuzzy-moose/tana/internal/local/metadata"
)

type Provider struct{}

var _ metadata.Provider = Provider{}

func (Provider) Read(ctx context.Context, source metadata.Source) (metadata.Values, []error) {
	for _, name := range source.Files {
		if path.Base(name) != "galleryinfo.txt" {
			continue
		}
		f, err := source.FS.Open(name)
		if err != nil {
			return metadata.Values{}, []error{fmt.Errorf("%s: %w", name, err)}
		}
		doc, diagnostics := Parse(contextReader{ctx, f})
		if err := f.Close(); err != nil {
			diagnostics = append(diagnostics, err)
		}
		for i, diagnostic := range diagnostics {
			diagnostics[i] = fmt.Errorf("%s: %w", name, diagnostic)
		}
		return metadata.Values{Title: doc.Title, Tags: doc.Tags}, diagnostics
	}
	return metadata.Values{}, nil
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
