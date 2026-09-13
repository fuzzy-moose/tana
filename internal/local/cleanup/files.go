package cleanup

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/cleanup/dbgen"
)

type snapshot struct {
	row       dbgen.ListSourcesRow
	public    Source
	file      os.FileInfo
	root      os.FileInfo
	canonical string
	reason    string
}

func inspect(row dbgen.ListSourcesRow) snapshot {
	v := snapshot{row: row, public: Source{
		ID: row.ID, LibraryID: row.LibraryID, LibraryName: row.LibraryName,
		Path: filepath.Join(row.LibraryPath, filepath.FromSlash(row.Path)), PandaID: sourcePandaID(row),
	}}
	if !filepath.IsAbs(row.LibraryPath) || !fs.ValidPath(row.Path) || strings.ContainsAny(row.Path, "\\\x00") || (len(row.Path) > 1 && row.Path[1] == ':') {
		v.reason = "Source path is outside its library"
		return v
	}
	// Count aliases even when this particular catalog entry is a symlink or
	// has a mismatched kind; deleting its target would still affect that entry.
	v.canonical, _ = filepath.EvalSymlinks(v.public.Path)
	root, err := os.OpenRoot(row.LibraryPath)
	if err != nil {
		v.reason = "Library is unavailable"
		return v
	}
	defer root.Close()
	v.root, err = root.Stat(".")
	if err == nil {
		v.file, err = root.Lstat(filepath.FromSlash(row.Path))
	}
	if err != nil {
		v.reason = "Source is unavailable or outside its library"
		return v
	}
	if (row.Kind == "archive" && !v.file.Mode().IsRegular()) || (row.Kind == "directory" && !v.file.IsDir()) {
		v.reason = "Source is no longer the cataloged file type"
		return v
	}
	if v.canonical == "" {
		v.reason = "Source is unavailable"
	}
	return v
}

func sourcePandaID(row dbgen.ListSourcesRow) int64 {
	name := row.Path
	if row.Kind == "directory" && name == "." {
		name = filepath.Base(row.LibraryPath)
	}
	return pandaID(name, row.Kind)
}

func sameAssociation(a, b dbgen.ListSourcesRow) bool {
	return a.ID == b.ID && a.LibraryID == b.LibraryID && a.LibraryPath == b.LibraryPath && a.Path == b.Path && a.Kind == b.Kind
}

func unchanged(before, after snapshot) bool {
	return after.reason == "" && sameAssociation(before.row, after.row) &&
		before.canonical == after.canonical && os.SameFile(before.root, after.root) && os.SameFile(before.file, after.file)
}

func filepathCanonical(row dbgen.ListSourcesRow) (string, error) {
	return filepath.EvalSymlinks(filepath.Join(row.LibraryPath, filepath.FromSlash(row.Path)))
}

func (s *Service) resolveCanonical(ctx context.Context, row dbgen.ListSourcesRow) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, s.probeTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	select {
	case s.probeSlots <- struct{}{}:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	type result struct {
		path string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		// Keep the slot until the syscall returns, even if its caller times out.
		// Stalled NAS calls must neither accumulate unbounded work nor use the DB.
		defer func() { <-s.probeSlots }()
		if err := ctx.Err(); err != nil {
			done <- result{err: err}
			return
		}
		path, err := s.canonicalPath(row)
		done <- result{path: path, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-done:
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return value.path, value.err
	}
}

func removeArchive(value snapshot) error {
	root, err := os.OpenRoot(value.row.LibraryPath)
	if err != nil {
		return err
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(rootInfo, value.root) {
		return errors.New("library changed since review")
	}
	info, err := root.Lstat(filepath.FromSlash(value.row.Path))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !os.SameFile(info, value.file) || info.Size() != value.file.Size() || !info.ModTime().Equal(value.file.ModTime()) {
		return errors.New("archive changed since review")
	}
	return root.Remove(filepath.FromSlash(value.row.Path))
}
