package galleryinfo

import (
	"testing"
	"testing/fstest"

	"github.com/fuzzy-moose/tana/internal/local/metadata"
)

func TestProviderUsesFirstExactBasenameInInventoryOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"GalleryInfo.txt":          {Data: []byte("Title: wrong case\nfooter")},
		"galleryinfo-1.txt":        {Data: []byte("Title: wrong name\nfooter")},
		"z/nested/galleryinfo.txt": {Data: []byte("Title: first\nTags: all ages\nfooter")},
		"galleryinfo.txt":          {Data: []byte("Title: root\nfooter")},
	}
	p := Provider{}
	values, diagnostics := p.Read(t.Context(), metadata.Source{Files: []string{"GalleryInfo.txt", "galleryinfo-1.txt", "z/nested/galleryinfo.txt", "galleryinfo.txt"}, FS: fsys})
	if len(diagnostics) != 0 || values.Title != "first" || len(values.Tags) != 1 {
		t.Fatalf("first encountered match: %+v, %v", values, diagnostics)
	}
	values, diagnostics = p.Read(t.Context(), metadata.Source{Files: []string{"GalleryInfo.txt", "galleryinfo-1.txt"}, FS: fsys})
	if len(diagnostics) != 0 || values.Title != "" || len(values.Tags) != 0 {
		t.Fatalf("missing file should be silent: %+v, %v", values, diagnostics)
	}
	values, diagnostics = p.Read(t.Context(), metadata.Source{Files: []string{"missing/galleryinfo.txt", "galleryinfo.txt"}, FS: fsys})
	if len(diagnostics) != 1 || values.Title != "" {
		t.Fatalf("unreadable first match must not fall through: %+v, %v", values, diagnostics)
	}
}
