package gallery

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fuzzy-moose/tana/internal/local/gallery/dbgen"
	"github.com/fuzzy-moose/tana/internal/local/source"
)

var ErrImageUnavailable = errors.New("page image unavailable")

type Summary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	PageCount int64  `json:"page_count"`
}

type Listing struct {
	Items    []Summary `json:"items"`
	Total    int64     `json:"total"`
	Page     int64     `json:"page"`
	PageSize int64     `json:"page_size"`
}

func (r *SQLiteRepository) Browse(ctx context.Context, search string, page, pageSize int64) (Listing, error) {
	result := Listing{Items: []Summary{}, Page: page, PageSize: pageSize}
	// Keep counts and the selected page consistent while a scan adds galleries.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	q := r.queries.WithTx(tx)
	result.Total, err = q.CountGallerySummaries(ctx, search)
	if err != nil {
		return result, err
	}
	result.Page = min(max(1, page), max(1, (result.Total+pageSize-1)/pageSize))
	rows, err := q.ListGallerySummaries(ctx, dbgen.ListGallerySummariesParams{
		Search: search, PageSize: pageSize, PageOffset: (result.Page - 1) * pageSize,
	})
	if err != nil {
		return result, err
	}
	for _, row := range rows {
		result.Items = append(result.Items, Summary{ID: row.ID, Title: row.Title, PageCount: row.PageCount})
	}
	return result, tx.Commit()
}

func (r *SQLiteRepository) Summary(ctx context.Context, id string) (Summary, error) {
	row, err := r.queries.GetGallerySummary(ctx, id)
	return Summary{ID: row.ID, Title: row.Title, PageCount: row.PageCount}, domainError(err)
}

type Image struct {
	io.ReadCloser
	ContentType string
	Size        int64
}

// OpenImage resolves a gallery occurrence through its source, never accepting a
// filesystem path from a client. Rooted opens also contain symlink replacements.
func (r *SQLiteRepository) OpenImage(ctx context.Context, id string, number int64) (Image, error) {
	if number < 1 {
		return Image{}, ErrNotFound
	}
	location, err := r.queries.GetGalleryImageLocation(ctx, dbgen.GetGalleryImageLocationParams{GalleryID: id, Offset: number - 1})
	if err != nil {
		return Image{}, domainError(err)
	}
	contentType := imageContentType(location.FilePath)
	if contentType == "" {
		return Image{}, ErrImageUnavailable
	}
	root, err := os.OpenRoot(location.LibraryPath)
	if err != nil {
		return Image{}, errors.Join(ErrImageUnavailable, err)
	}
	defer root.Close()
	name := filepath.FromSlash(location.SourcePath)
	if source.Kind(location.Kind) == source.Directory {
		name = filepath.Join(name, filepath.FromSlash(location.FilePath))
	}
	f, err := root.Open(name)
	if err != nil {
		return Image{}, errors.Join(ErrImageUnavailable, err)
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return Image{}, errors.Join(ErrImageUnavailable, err)
	}
	if source.Kind(location.Kind) == source.Directory {
		return Image{ReadCloser: f, ContentType: contentType, Size: info.Size()}, nil
	}
	archive, err := zip.NewReader(f, info.Size())
	if err == nil {
		for _, entry := range archive.File {
			if entry.Name == location.FilePath && entry.Mode().IsRegular() {
				var reader io.ReadCloser
				reader, err = entry.Open()
				if err == nil {
					return Image{ReadCloser: &archiveImage{ReadCloser: reader, archive: f}, ContentType: contentType, Size: int64(entry.UncompressedSize64)}, nil
				}
				break
			}
		}
	}
	f.Close()
	return Image{}, errors.Join(ErrImageUnavailable, err)
}

type archiveImage struct {
	io.ReadCloser
	archive *os.File
}

func (f *archiveImage) Close() error {
	return errors.Join(f.ReadCloser.Close(), f.archive.Close())
}

func imageContentType(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}
