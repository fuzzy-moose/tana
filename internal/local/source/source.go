// Package source owns the complete file inventory of sources in libraries.
package source

import (
	"errors"
	"io/fs"
	"path"
	"strings"
)

type Kind string

const (
	Directory Kind = "directory"
	Archive   Kind = "archive"
)

var (
	ErrNotFound      = errors.New("source not found")
	ErrInvalidSource = errors.New("invalid source")
	ErrInvalidPath   = errors.New("path must be a clean relative path")
)

type Source struct {
	ID        int64
	LibraryID int64
	// Path is slash-separated and relative to the library root. A directory
	// source at the library root uses ".".
	Path string
	Kind Kind
}

type File struct {
	ID       int64
	SourceID int64
	// Path is relative to the source directory or the archive root.
	Path string
}

func IsArchive(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".cbz", ".zip":
		return true
	default:
		return false
	}
}

// validPath accepts portable, slash-separated paths without traversal. The
// caller converts native filesystem paths before cataloging them.
func validPath(name string) bool {
	return fs.ValidPath(name) && !strings.ContainsAny(name, "\\\x00") &&
		!(len(name) >= 2 && name[1] == ':')
}

func validate(sourcePath string, kind Kind, files []string) error {
	if !validPath(sourcePath) {
		return ErrInvalidPath
	}
	switch kind {
	case Archive:
		if !IsArchive(sourcePath) {
			return ErrInvalidSource
		}
	case Directory:
		if len(files) == 0 {
			return ErrInvalidSource
		}
	default:
		return ErrInvalidSource
	}
	seen := make(map[string]bool, len(files))
	for _, name := range files {
		if name == "." || !validPath(name) {
			return ErrInvalidPath
		}
		if seen[name] || (kind == Directory && (strings.Contains(name, "/") || IsArchive(name))) {
			return ErrInvalidSource
		}
		seen[name] = true
	}
	return nil
}
