package tag_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/gallery"
	"github.com/fuzzy-moose/tana/internal/local/storage"
	"github.com/fuzzy-moose/tana/internal/local/tag"
)

func TestSharedVocabularyNormalizationAndAtomicAssignments(t *testing.T) {
	ctx := t.Context()
	db, _, err := storage.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	galleries := gallery.NewSQLiteRepository(db)
	a, err := galleries.Create(ctx, "A", nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := galleries.Create(ctx, "B", nil)
	if err != nil {
		t.Fatal(err)
	}
	repo := tag.NewSQLiteRepository(db)
	assign := func(id int64, values ...tag.Value) error {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if err := repo.ReplaceForGalleryTx(ctx, tx, id, values); err != nil {
			return err
		}
		return tx.Commit()
	}
	if err := assign(a.ID, tag.Value{Namespace: " LANGUAGE ", Value: "English.v2"}, tag.Value{Namespace: "language", Value: "english.v2"}); err != nil {
		t.Fatal(err)
	}
	if err := assign(b.ID, tag.Value{Namespace: "language", Value: "english.v2"}); err != nil {
		t.Fatal(err)
	}
	at, err := repo.ListForGallery(ctx, a.ID)
	if err != nil || len(at) != 1 || at[0].Namespace.Name != "language" || at[0].Value != "english.v2" {
		t.Fatalf("normalized tags: %+v, %v", at, err)
	}
	if at[0].ID != 1 || at[0].Namespace.ID != 1 {
		t.Fatalf("first vocabulary IDs: %+v", at[0])
	}
	bt, err := repo.ListForGallery(ctx, b.ID)
	if err != nil || !reflect.DeepEqual(at, bt) {
		t.Fatalf("shared identities differ: %+v, %+v, %v", at, bt, err)
	}
	if err := assign(a.ID, tag.Value{Namespace: "new", Value: "valid"}, tag.Value{Namespace: "new", Value: "not_valid"}); !errors.Is(err, tag.ErrInvalid) {
		t.Fatalf("invalid catalog write: %v", err)
	}
	after, err := repo.ListForGallery(ctx, a.ID)
	if err != nil || !reflect.DeepEqual(at, after) {
		t.Fatalf("failed write changed assignments: %+v, %v", after, err)
	}
	var count int
	if err := db.QueryRow("SELECT count(*) FROM namespaces").Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback leaked namespace: %d, %v", count, err)
	}
	if err := galleries.Delete(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT count(*) FROM gallery_tags").Scan(&count); err != nil || count != 1 {
		t.Fatalf("gallery deletion retained assignments: %d, %v", count, err)
	}
	bt, err = repo.ListForGallery(ctx, b.ID)
	if err != nil || !reflect.DeepEqual(at, bt) {
		t.Fatalf("gallery deletion removed shared tag: %+v, %v", bt, err)
	}
	for _, name := range []string{"a.b", "a-b", "a b", "a1"} {
		if _, err := db.Exec("INSERT INTO namespaces (name) VALUES (?)", name); err == nil {
			t.Errorf("database accepted invalid namespace %q", name)
		}
	}
}

func TestTagVocabulary(t *testing.T) {
	v, err := tag.Normalize(tag.Value{Namespace: " MyCategory ", Value: " Multi-Work Series 2.0 "})
	if err != nil || v.Namespace != "mycategory" || v.Value != "multi-work series 2.0" {
		t.Fatalf("normalization: %+v, %v", v, err)
	}
	for _, value := range []string{"", "  ", "a_b", "a:b", "a,b", "日本語", "K", "two\twords"} {
		for _, v := range []tag.Value{{Namespace: "other", Value: value}, {Namespace: value, Value: "tag"}} {
			if _, err := tag.Normalize(v); !errors.Is(err, tag.ErrInvalid) {
				t.Errorf("invalid vocabulary accepted: %+v, %v", v, err)
			}
		}
	}
	for _, namespace := range []string{"a.b", "a-b", "a b", "a1"} {
		if _, err := tag.Normalize(tag.Value{Namespace: namespace, Value: "tag"}); !errors.Is(err, tag.ErrInvalid) {
			t.Errorf("invalid namespace accepted: %q, %v", namespace, err)
		}
	}
}
