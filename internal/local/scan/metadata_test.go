package scan

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fuzzy-moose/tana/internal/cleanup"
	"github.com/fuzzy-moose/tana/internal/local/tag"
)

func metadataFile(t *testing.T, root, name, contents string) {
	t.Helper()
	writeFile(t, root, name)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func metadataArchive(t *testing.T, root string) {
	t.Helper()
	f, err := os.Create(filepath.Join(root, "archive.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup.CloseAndLog(f, slog.Default())
	w := zip.NewWriter(f)
	for _, entry := range []struct{ name, content string }{
		{"top/1.jpg", "image"},
		{"z/galleryinfo.txt", "Title: First nested\nTags: language:ENGLISH, all ages\nArbitrary footer\n"},
		{"galleryinfo.txt", "Title: Root ignored\nTags: other:ignored\nfooter\n"},
	} {
		writer, err := w.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestImportGalleryMetadataFromDirectoryAndNestedArchive(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeFile(t, l.Path, "directory/1.jpg")
	metadataFile(t, l.Path, "directory/galleryinfo.txt", "Title: Directory title\nUpload Time: 2020-01-01 12:00\nUploaded By: uploader\nDownloaded: 2020-02-01 12:00\nTags: LANGUAGE:English, all ages, language:english\nUploader's Comments:\n\nraw text\nfooter\n")
	metadataArchive(t, l.Path)
	writeFile(t, l.Path, "missing/1.jpg")
	writeFile(t, l.Path, "wrongcase/1.jpg")
	metadataFile(t, l.Path, "wrongcase/GalleryInfo.txt", "Title: ignored\nfooter")
	metadataFile(t, l.Path, "notes/galleryinfo.txt", "Title: No gallery\nTags: unused:tag\nfooter")
	var logs bytes.Buffer
	f.logger = slog.New(slog.NewTextHandler(&logs, nil))
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	status := awaitFinished(t, s)
	if status.Imported != 5 || status.GalleriesCreated != 4 || status.Phase != "completed" {
		t.Fatalf("import: %+v", status)
	}
	if logs.Len() != 0 {
		t.Fatalf("missing metadata should be silent: %s", logs.String())
	}
	galleries, err := f.galleries.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var shared []tag.Tag
	for _, g := range galleries {
		if g.SourceID == 0 {
			t.Fatalf("automatic gallery missing source link: %+v", g)
		}
		imported, err := f.sources.Get(t.Context(), g.SourceID)
		if err != nil {
			t.Fatal(err)
		}
		wantTitle := map[string]string{"directory": "Directory title", "archive.cbz": "First nested", "missing": "missing", "wrongcase": "wrongcase"}[imported.Path]
		if g.Title != wantTitle {
			t.Fatalf("%s title: %q, want %q", imported.Path, g.Title, wantTitle)
		}
		tags, err := tag.NewSQLiteRepository(f.db).ListForGallery(t.Context(), g.ID)
		if err != nil {
			t.Fatal(err)
		}
		if imported.Path == "directory" || imported.Path == "archive.cbz" {
			if len(tags) != 2 || tags[0].Namespace.Name != "language" || tags[0].Value != "english" || tags[1].Namespace.Name != "other" || tags[1].Value != "all ages" {
				t.Fatalf("imported tags: %+v", tags)
			}
			if shared != nil && (shared[0] != tags[0] || shared[1] != tags[1]) {
				t.Fatalf("provider created duplicate tag identities: %+v, %+v", shared, tags)
			}
			shared = tags
		} else if len(tags) != 0 {
			t.Fatalf("unexpected tags: %+v", tags)
		}
	}
	var tags int
	if err := f.db.QueryRow("SELECT count(*) FROM tags").Scan(&tags); err != nil || tags != 2 {
		t.Fatalf("source without gallery stored metadata: %d, %v", tags, err)
	}
	metadataFile(t, l.Path, "directory/galleryinfo.txt", "Title: Changed later\nTags: new:tag\nfooter")
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Discovered != 0 {
		t.Fatalf("metadata reapplied on rescan: %+v", status)
	}
	if err := f.libraries.Delete(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	var galleriesLeft, assignments int
	if err := f.db.QueryRow("SELECT (SELECT count(*) FROM galleries), (SELECT count(*) FROM gallery_tags)").Scan(&galleriesLeft, &assignments); err != nil || galleriesLeft != 0 || assignments != 0 {
		t.Fatalf("linked gallery deletion: galleries=%d assignments=%d, %v", galleriesLeft, assignments, err)
	}
}

func TestMetadataFailuresLogWithoutFailingImport(t *testing.T) {
	for _, unreadable := range []bool{false, true} {
		name := "malformed"
		if unreadable {
			name = "unreadable"
		}
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			l := f.library(t, "Comics")
			writeFile(t, l.Path, "book/1.jpg")
			metadataFile(t, l.Path, "book/galleryinfo.txt", "Title: \nUpload Time: invalid\nTags: language:ENGLISH, invalid_tag\nfooter")
			var logs bytes.Buffer
			f.logger = slog.New(slog.NewTextHandler(&logs, nil))
			s := f.scanner(t, func(root string) fs.FS {
				if unreadable {
					return unreadableMetadataFS{os.DirFS(root)}
				}
				return os.DirFS(root)
			})
			if err := s.Request(t.Context(), l.ID); err != nil {
				t.Fatal(err)
			}
			if status := awaitFinished(t, s); status.Phase != "completed" || status.Imported != 1 || status.GalleriesCreated != 1 {
				t.Fatalf("metadata failure blocked import: %+v", status)
			}
			galleries, err := f.galleries.List(t.Context())
			if err != nil || len(galleries) != 1 || galleries[0].Title != "book" {
				t.Fatalf("fallback title: %+v, %v", galleries, err)
			}
			tags, err := tag.NewSQLiteRepository(f.db).ListForGallery(t.Context(), galleries[0].ID)
			want := 1
			if unreadable {
				want = 0
			}
			if err != nil || len(tags) != want {
				t.Fatalf("partial tags: %+v, %v", tags, err)
			}
			if !strings.Contains(logs.String(), "source_metadata_failed") || !strings.Contains(logs.String(), "galleryinfo.txt") {
				t.Fatalf("missing diagnostic context: %s", logs.String())
			}
		})
	}
}

type unreadableMetadataFS struct{ fs.FS }

func (f unreadableMetadataFS) Open(name string) (fs.File, error) {
	if strings.HasSuffix(name, "/galleryinfo.txt") {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestTagPersistenceFailureRollsBackImportAndRetries(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeFile(t, l.Path, "1.jpg")
	metadataFile(t, l.Path, "galleryinfo.txt", "Title: Metadata\nTags: language:english\nfooter")
	if _, err := f.db.Exec(`CREATE TRIGGER fail_tag BEFORE INSERT ON gallery_tags BEGIN SELECT RAISE(ABORT, 'test tag failure'); END`); err != nil {
		t.Fatal(err)
	}
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Imported != 0 || status.FailedSources != 1 {
		t.Fatalf("failed tag write: %+v", status)
	}
	var count int
	if err := f.db.QueryRow(`SELECT (SELECT count(*) FROM sources) + (SELECT count(*) FROM source_files) + (SELECT count(*) FROM galleries) + (SELECT count(*) FROM gallery_pages) + (SELECT count(*) FROM namespaces) + (SELECT count(*) FROM tags) + (SELECT count(*) FROM gallery_tags)`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial import survived: %d, %v", count, err)
	}
	if _, err := f.db.Exec("DROP TRIGGER fail_tag"); err != nil {
		t.Fatal(err)
	}
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Imported != 1 || status.GalleriesCreated != 1 || status.Phase != "completed" {
		t.Fatalf("metadata retry: %+v", status)
	}
}
