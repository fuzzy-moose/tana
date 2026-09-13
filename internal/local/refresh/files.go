package refresh

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/refresh/dbgen"
)

type observation struct {
	missing   map[int64]bool
	uncertain map[int64]string
	blocked   string
}

type directory struct {
	entries map[string]fs.DirEntry
	err     error
}

func inspect(ctx context.Context, filesystem fs.FS, rows []dbgen.ListSourcesRow) observation {
	result := observation{missing: map[int64]bool{}, uncertain: map[int64]string{}}
	directories := map[string]directory{}
	read := func(name string) directory {
		if err := ctx.Err(); err != nil {
			return directory{err: err}
		}
		if cached, ok := directories[name]; ok {
			return cached
		}
		entries, err := fs.ReadDir(filesystem, name)
		d := directory{entries: map[string]fs.DirEntry{}, err: err}
		for _, entry := range entries {
			d.entries[entry.Name()] = entry
		}
		directories[name] = d
		return d
	}
	if read(".").err != nil {
		result.blocked = "Library root is unavailable or unreadable."
		return result
	}
	present := 0
	for _, row := range rows {
		if ctx.Err() != nil {
			return observation{blocked: "Storage check timed out or was canceled."}
		}
		if !fs.ValidPath(row.Path) || strings.ContainsAny(row.Path, "\\\x00") || (len(row.Path) > 1 && row.Path[1] == ':') {
			result.uncertain[row.ID] = "Source path cannot be checked safely."
			continue
		}
		if row.Path != "." {
			parent := "."
			for _, part := range strings.Split(row.Path, "/") {
				d := read(parent)
				if d.err != nil {
					result.uncertain[row.ID] = "A containing directory is unavailable or unreadable."
					break
				}
				// Only a complete, successful directory listing proves absence.
				// An existing path is retained regardless of its contents or type.
				if _, exists := d.entries[part]; !exists {
					result.missing[row.ID] = true
					break
				}
				parent = path.Join(parent, part)
			}
		}
		if !result.missing[row.ID] && result.uncertain[row.ID] == "" {
			present++
		}
	}
	if len(rows) > 0 && present == 0 {
		result.blocked = "No cataloged source could be confirmed present. Storage may be disconnected; removal is blocked for this library."
	}
	return result
}

func (s *Service) probe(ctx context.Context, rows []dbgen.ListSourcesRow) observation {
	ctx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()
	if len(rows) == 0 {
		return observation{}
	}
	timeout := observation{blocked: "Storage check timed out or was canceled. Removal is blocked for this library."}
	select {
	case s.probeSlots <- struct{}{}:
	case <-ctx.Done():
		return timeout
	}
	done := make(chan observation, 1)
	go func() {
		// Retain the slot until filesystem work returns, including after a NAS
		// timeout. Late observations never write to the catalog.
		defer func() { <-s.probeSlots }()
		if ctx.Err() != nil {
			return
		}
		done <- inspect(ctx, s.dirFS(rows[0].LibraryPath), rows)
	}()
	select {
	case <-ctx.Done():
		return timeout
	case result := <-done:
		if ctx.Err() != nil {
			return timeout
		}
		return result
	}
}
