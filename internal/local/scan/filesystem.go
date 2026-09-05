package scan

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/metadata"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

type candidate struct {
	libraryID int64
	root      fs.FS
	path      string
	kind      source.Kind
}

func (s *Service) discover(ctx context.Context, libraries []library.Library) ([]candidate, error) {
	var candidates []candidate
	for _, l := range libraries {
		registered, err := s.sources.List(ctx, l.ID)
		if err != nil {
			return nil, err
		}
		known := make(map[string]bool, len(registered))
		for _, existing := range registered {
			known[existing.Path] = true
		}
		root := s.dirFS(l.Path)
		add := func(name string, kind source.Kind) {
			if !known[name] {
				candidates = append(candidates, candidate{libraryID: l.ID, root: root, path: name, kind: kind})
				s.mu.Lock()
				s.status.Discovered++
				s.mu.Unlock()
			}
		}
		directories := []string{"."}
		for len(directories) > 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			name := directories[len(directories)-1]
			directories = directories[:len(directories)-1]
			entries, err := fs.ReadDir(root, name)
			if err != nil {
				s.mu.Lock()
				s.status.DiscoveryErrors++
				s.mu.Unlock()
				s.logger.Error("scan_discovery_failed", "library_id", l.ID, "path", name, "error", err)
				continue
			}
			hasDirectory, hasArchive, hasFile := false, false, false
			for _, entry := range entries {
				child := path.Join(name, entry.Name())
				if entry.IsDir() {
					hasDirectory = true
					directories = append(directories, child)
				} else if entry.Type().IsRegular() {
					hasFile = true
					if source.IsArchive(entry.Name()) {
						hasArchive = true
						add(child, source.Archive)
					}
				}
			}
			if hasFile && !hasDirectory && !hasArchive {
				add(name, source.Directory)
			}
		}
	}
	return candidates, ctx.Err()
}

// prepareSource reads inventories and metadata outside the import transaction,
// retaining encounter order and keeping archives open only during preparation.
func (s *Service) prepareSource(ctx context.Context, c candidate) preparedSource {
	result := preparedSource{candidate: c}
	if c.kind == source.Directory {
		entries, err := fs.ReadDir(c.root, c.path)
		if err != nil {
			result.err = err
			return result
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				result.files = append(result.files, entry.Name())
			}
		}
		root, err := fs.Sub(c.root, c.path)
		if err != nil {
			result.err = err
			return result
		}
		return s.readMetadata(ctx, result, root)
	}
	f, err := c.root.Open(c.path)
	if err != nil {
		result.err = err
		return result
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		result.err = err
		return result
	}
	reader, ok := f.(io.ReaderAt)
	if !ok {
		result.err = fmt.Errorf("archive %s does not support random reads", c.path)
		return result
	}
	archive, err := zip.NewReader(reader, info.Size())
	if err != nil {
		result.err = err
		return result
	}
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			result.err = err
			return result
		}
		if entry.Mode().IsRegular() {
			result.files = append(result.files, entry.Name)
		}
	}
	return s.readMetadata(ctx, result, archive)
}

func (s *Service) readMetadata(ctx context.Context, result preparedSource, root fs.FS) preparedSource {
	var diagnostics []error
	result.metadata, diagnostics = s.provider.Read(ctx, metadata.Source{Files: result.files, FS: root})
	for _, diagnostic := range diagnostics {
		s.logger.Warn("source_metadata_failed", "library_id", result.candidate.libraryID, "path", result.candidate.path, "provider", "galleryinfo", "error", diagnostic)
	}
	result.err = ctx.Err()
	return result
}
