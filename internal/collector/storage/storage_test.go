package storage

import (
	"bytes"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenBackfillsFeedSightingsFromRetainedCaptures(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ddl, err := migrations.ReadFile("migrations/0001_feeds.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatal(err)
	}
	body := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><link href="https://example.test/g/1/a/"/></entry><entry><link href="https://example.test/g/1/a/"/></entry></feed>`)
	if _, err := db.Exec(`INSERT INTO gallery_refs VALUES (1, 'a');
		INSERT INTO raw_feeds (captured_at, feed_url, body, processed_at) VALUES
		(100, 'https://example.test/feed', ?, 101), (200, 'https://example.test/feed', ?, 201),
		(300, 'https://example.test/feed', '<malformed', NULL);
		INSERT INTO feed_continuity_checks VALUES (1, 2, 'overlap', 200, 201);
		PRAGMA user_version = 1;`, body, body); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopening twice must retain the backfill, processed state, and raw bytes.
	for range 2 {
		db, _, err = Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		var sightings, refs, pending int
		if err := db.QueryRow(`SELECT (SELECT count(*) FROM feed_gallery_refs),
			(SELECT count(*) FROM gallery_refs),
			(SELECT count(*) FROM raw_feeds WHERE processed_at IS NULL)`).Scan(&sightings, &refs, &pending); err != nil {
			t.Fatal(err)
		}
		if sightings != 2 || refs != 1 || pending != 1 {
			t.Fatalf("sightings=%d refs=%d pending=%d", sightings, refs, pending)
		}
		var saved []byte
		var processed int64
		if err := db.QueryRow(`SELECT body, processed_at FROM raw_feeds WHERE id = 1`).Scan(&saved, &processed); err != nil || !bytes.Equal(saved, body) || processed != 101 {
			t.Fatalf("capture changed: %q, %d, %v", saved, processed, err)
		}
		var status string
		if err := db.QueryRow(`SELECT status FROM feed_continuity_checks`).Scan(&status); err != nil || status != "overlap" {
			t.Fatalf("continuity changed: %s, %v", status, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
