package scan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/local/enrichment"
)

func TestScanQueuesOnlyNewMatchingGalleriesAtomically(t *testing.T) {
	f := setup(t)
	l := f.library(t, "Comics")
	writeArchive(t, l.Path, "Existing [10].zip", "1.jpg")
	s := f.scanner(t, os.DirFS)
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	awaitFinished(t, s)
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM panda_enrichments`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("disabled enrichment queued work: %d, %v", count, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Inspect durable import work independently of the collector worker.
	enrich := enrichment.New(ctx, f.db, nil, f.logger)
	t.Cleanup(enrich.Close)
	s.enrichment = enrich
	writeArchive(t, l.Path, "New [20].cbz", "1.jpg")
	if _, err := f.db.Exec(`CREATE TRIGGER fail_enqueue BEFORE INSERT ON panda_enrichments BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Request(t.Context(), l.ID); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.FailedSources != 1 {
		t.Fatalf("enqueue failure: %+v", status)
	}
	if err := f.db.QueryRow(`SELECT count(*) FROM sources`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("import escaped rollback: %d, %v", count, err)
	}
	if _, err := f.db.Exec(`DROP TRIGGER fail_enqueue`); err != nil {
		t.Fatal(err)
	}
	writeFile(t, l.Path, "Directory [30]/1.jpg")
	metadataFile(t, l.Path, "Directory [30]/galleryinfo.txt", "Title: Different name [999]\nfooter")
	writeFile(t, l.Path, "No images [40]/notes.txt")
	writeFile(t, l.Path, "Wrong suffix [50] extra/1.jpg")
	root := filepath.Join(t.TempDir(), "Root source [60]")
	writeFile(t, root, "1.jpg")
	if _, err := f.libraries.Create(t.Context(), "Root source", root); err != nil {
		t.Fatal(err)
	}
	if err := s.Request(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Imported != 5 || status.GalleriesCreated != 4 || status.Phase != "completed" {
		t.Fatalf("import: %+v", status)
	}
	rows, err := f.db.Query(`SELECT panda_id FROM panda_enrichments ORDER BY panda_id`)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if !reflect.DeepEqual(ids, []int64{20, 30, 60}) {
		t.Fatalf("queued IDs: %v", ids)
	}
	if err := s.Request(t.Context(), 0); err != nil {
		t.Fatal(err)
	}
	if status := awaitFinished(t, s); status.Discovered != 0 {
		t.Fatalf("rescan: %+v", status)
	}
}
