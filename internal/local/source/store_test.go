package source

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/library"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestInventoryPersistsAllFiles(t *testing.T) {
	ctx := t.Context()
	db := pgtest.Open(t, storage.Migrations())
	libraries := library.NewStore(db)
	l, err := libraries.Create(ctx, "Comics", filepath.Join(t.TempDir(), "offline"))
	if err != nil {
		t.Fatal(err)
	}
	r := NewStore(db)
	s, err := r.Create(ctx, l.ID, "Series/Volume.CBZ", Archive, []string{"chapter/2.JPG", "notes.txt", "nested/book.zip", "chapter/10.png"})
	if err != nil {
		t.Fatal(err)
	}
	files, err := r.Files(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 4 {
		t.Fatalf("inventory: %+v", files)
	}
	for _, file := range files {
		if file.ID == 0 || file.SourceID != s.ID {
			t.Fatalf("file: %+v", file)
		}
	}
	if _, err := r.Create(ctx, l.ID, s.Path, Archive, []string{"other.png"}); err == nil {
		t.Fatal("duplicate source path accepted")
	}
	if _, err := r.Create(ctx, 999, "book.zip", Archive, []string{"1.jpg"}); err == nil {
		t.Fatal("missing library accepted")
	}
	db = pgtest.Reopen(t, db, storage.Migrations())
	r = NewStore(db)
	got, err := r.Get(ctx, s.ID)
	if err != nil || got != s {
		t.Fatalf("reopened source: %+v, %v", got, err)
	}
	gotFiles, err := r.Files(ctx, s.ID)
	if err != nil || !reflect.DeepEqual(files, gotFiles) {
		t.Fatalf("reopened files: %+v, %v", gotFiles, err)
	}
	listed, err := r.List(ctx, l.ID)
	if err != nil || len(listed) != 1 || listed[0] != s {
		t.Fatalf("sources: %+v, %v", listed, err)
	}
}

func TestSourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		kind       Kind
		files      []string
		want       error
	}{
		{"leaf with images", "Volume", Directory, []string{"1.jpg", "notes.txt"}, nil},
		{"unsupported files still qualify", "Volume", Directory, []string{"notes.txt"}, nil},
		{"library root source", ".", Directory, []string{"1.jpg"}, nil},
		{"empty directory", "Volume", Directory, nil, ErrInvalidSource},
		{"subdirectory", "Volume", Directory, []string{"chapter/1.jpg"}, ErrInvalidSource},
		{"archive wins", "Volume", Directory, []string{"1.jpg", "book.ZIP"}, ErrInvalidSource},
		{"empty archive", "book.zip", Archive, nil, nil},
		{"nested archive is a file", "book.cbz", Archive, []string{"inner.zip", "chapter/1.jpg"}, nil},
		{"unsupported archive", "book.rar", Archive, []string{"1.jpg"}, ErrInvalidSource},
		{"duplicate path", "book.zip", Archive, []string{"1.jpg", "1.jpg"}, ErrInvalidSource},
		{"source traversal", "../book.zip", Archive, nil, ErrInvalidPath},
		{"file traversal", "book.zip", Archive, []string{"../1.jpg"}, ErrInvalidPath},
		{"absolute file", "book.zip", Archive, []string{"/1.jpg"}, ErrInvalidPath},
		{"native path", "book.zip", Archive, []string{"chapter\\1.jpg"}, ErrInvalidPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validate(tc.path, tc.kind, tc.files); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}
