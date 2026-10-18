package catalogfilter

import (
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/pgdb/pgtest"
)

func TestDefaultFilterSurvivesRestart(t *testing.T) {
	db := pgtest.Open(t, storage.Migrations())
	store := New(db)
	empty, err := store.Load(t.Context())
	if err != nil || empty.Query != "" || empty.Categories == nil || len(empty.Categories) != 0 {
		t.Fatalf("initial filter: %#v, %v", empty, err)
	}
	want, err := Normalize(Filter{Query: " -l:japanese$ ", Categories: []string{"Manga", "doujinshi", "manga"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), want); err != nil {
		t.Fatal(err)
	}
	db = pgtest.Reopen(t, db, storage.Migrations())
	got, err := New(db).Load(t.Context())
	if err != nil || !reflect.DeepEqual(got, want) || got.Query != "-l:japanese$" || len(got.Categories) != 2 {
		t.Fatalf("reopened filter: %#v, %v; want %#v", got, err, want)
	}
}
