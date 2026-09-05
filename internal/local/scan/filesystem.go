package scan

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

type candidate struct {
	libraryID string
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

// inventory reads the discovered source as it exists during import. It neither
// repeats discovery nor opens nested archives or decodes image contents.
func inventory(ctx context.Context, c candidate) ([]string, error) {
	var files []string
	if c.kind == source.Directory {
		entries, err := fs.ReadDir(c.root, c.path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				files = append(files, entry.Name())
			}
		}
		return files, ctx.Err()
	}
	f, err := c.root.Open(c.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	reader, ok := f.(io.ReaderAt)
	if !ok {
		return nil, fmt.Errorf("archive %s does not support random reads", c.path)
	}
	archive, err := zip.NewReader(reader, info.Size())
	if err != nil {
		return nil, err
	}
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.Mode().IsRegular() {
			files = append(files, entry.Name)
		}
	}
	return files, nil
}
