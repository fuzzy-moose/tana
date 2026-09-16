package storage

import (
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
	// Backfill sightings before discarding processed payloads; failed feeds survive.
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
		if err := db.QueryRow(`SELECT body, processed_at FROM raw_feeds WHERE id = 1`).Scan(&saved, &processed); err != nil || len(saved) != 0 || processed != 101 {
			t.Fatalf("processed capture cleanup: %q, %d, %v", saved, processed, err)
		}
		if err := db.QueryRow(`SELECT body FROM raw_feeds WHERE id = 3`).Scan(&saved); err != nil || string(saved) != "<malformed" {
			t.Fatalf("unprocessed capture changed: %q, %v", saved, err)
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

func TestOpenPreservesCompletedFavoritesAsIncrementalBaseline(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:5] {
		ddl, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (1, '123456789a');
 INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES (1, 'https://panda.test', '42', 2, 'Manga', 1000);
 INSERT INTO favorites (category_id, gallery_id, token, added_at) VALUES (1, 1, '123456789a', 500);
 PRAGMA user_version = 5;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, _, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var added, committed, synced, jobs int64
	if err := migrated.QueryRow(`SELECT f.added_at, f.committed_added_at, c.synced_at, (SELECT count(*) FROM favorite_syncs)
 FROM favorites f JOIN favorite_categories c ON c.id = f.category_id`).Scan(&added, &committed, &synced, &jobs); err != nil {
		t.Fatal(err)
	}
	if added != 500 || committed != 500 || synced != 1000 || jobs != 0 {
		t.Fatalf("added=%d committed=%d synced=%d jobs=%d", added, committed, synced, jobs)
	}
	var observed int
	var baseline string
	if err := migrated.QueryRow(`SELECT (SELECT count(*) FROM favorite_observations WHERE gallery_id = 1), baseline_state FROM favorite_download_settings`).Scan(&observed, &baseline); err != nil || observed != 1 || baseline != "not_started" {
		t.Fatalf("download history=%d baseline=%q %v", observed, baseline, err)
	}
}

func TestOpenKeepsLegacyBanOnlyForUnauthenticatedRequests(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:19] {
		ddl, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE panda_ban SET until_at = 120000;
 INSERT INTO favorite_categories (id, host, account_key, category, name, synced_at) VALUES (1, 'https://panda.test', '42', 2, 'Manga', 0);
 INSERT INTO favorite_syncs (category_id, state, full, queued_at, retry_at) VALUES (1, 'queued', 0, 1000, 130000);
 INSERT INTO panda_downloads (gallery_id, token, state, created_at, updated_at, retry_at) VALUES (1, '123456789a', 'queued', 1000, 1000, 140000);
 PRAGMA user_version = 19;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		migrated, _, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		var main, authenticated, favoritesRetry, downloadRetry int64
		err = migrated.QueryRow(`SELECT
 (SELECT until_at FROM panda_ban WHERE id = 1),
 (SELECT until_at FROM panda_ban WHERE id = 2),
 (SELECT retry_at FROM favorite_syncs WHERE category_id = 1),
 (SELECT retry_at FROM panda_downloads WHERE gallery_id = 1)`).Scan(&main, &authenticated, &favoritesRetry, &downloadRetry)
		if err := migrated.Close(); err != nil {
			t.Fatal(err)
		}
		if err != nil || main != 120000 || authenticated != 0 || favoritesRetry != 130000 || downloadRetry != 140000 {
			t.Fatalf("main=%d authenticated=%d favoritesRetry=%d downloadRetry=%d: %v", main, authenticated, favoritesRetry, downloadRetry, err)
		}
	}
}

func TestOpenBackfillsCategoryInExistingCatalogProjections(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:23] {
		ddl, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	for i, body := range []string{`{"category":" Manga "}`, `{"category":"Artist CG"}`, `{}`, `{"category":"unrecognized"}`} {
		if _, err := db.Exec("INSERT INTO gallery_refs (gallery_id, token) VALUES (?, 'token')", i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (?, ?, 0)", i+1, []byte(body)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO catalog_galleries
			(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, expunged)
			VALUES (?, 'title', 'title', '', '', 0, 0, 0)`, i+1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("PRAGMA user_version = 23"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	migrated, _, err := Open(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	for i, want := range []string{"manga", "artist cg", "", ""} {
		var got string
		if err := migrated.QueryRow("SELECT category FROM catalog_galleries WHERE gallery_id = ?", i+1).Scan(&got); err != nil || got != want {
			t.Fatalf("gallery %d category = %q, %v; want %q", i+1, got, err, want)
		}
	}
}

func TestOpenPreservesCatalogAndTagsWhenAddingProvisionalMetadata(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "collector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files[:26] {
		ddl, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token) VALUES (1, 'token1'), (2, 'token2');
		INSERT INTO gallery_metadata (gallery_id, body, refreshed_at) VALUES (1, '{}', 1000);
		INSERT INTO catalog_galleries
			(gallery_id, title, title_lower, title_japanese_lower, thumbnail_url, page_count, posted, expunged, category)
			VALUES (1, 'Retained title', 'retained title', '', 'thumbnail', 15, 500, 0, 'manga');
		INSERT INTO catalog_tags (id, namespace, value, value_lower) VALUES (1, 'artist', 'Retained artist', 'retained artist');
		INSERT INTO catalog_gallery_tags (gallery_id, tag_id) VALUES (1, 1);
		INSERT INTO raw_feeds (id, captured_at, feed_url, body, processed_at) VALUES (1, 2000, 'feed', X'', 2100), (2, 1000, 'feed', X'', 1100);
		INSERT INTO feed_gallery_refs (raw_feed_id, gallery_id) VALUES (1, 2), (2, 2);
		PRAGMA user_version = 26;`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		migrated, _, err := Open(t.Context(), dir)
		if err != nil {
			t.Fatal(err)
		}
		var title, tag, source, parent string
		var pages, posted, ordered, base, indexes int64
		err = migrated.QueryRow(`SELECT g.title, t.value, g.metadata_source, g.page_count, g.posted, g.ordered_at,
			(SELECT feed_metadata_base_at FROM gallery_refs WHERE gallery_id = 2),
			(SELECT "table" FROM pragma_foreign_key_list('catalog_galleries')),
			(SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name IN ('catalog_posted', 'catalog_visible_posted', 'catalog_category_posted', 'catalog_tag_galleries'))
			FROM catalog_galleries g JOIN catalog_gallery_tags gt ON gt.gallery_id = g.gallery_id
			JOIN catalog_tags t ON t.id = gt.tag_id WHERE g.gallery_id = 1`).Scan(&title, &tag, &source, &pages, &posted, &ordered, &base, &parent, &indexes)
		if err != nil || title != "Retained title" || tag != "Retained artist" || source != "collected" || pages != 15 || posted != 500 || ordered != 500 || base != 1000 || parent != "gallery_refs" || indexes != 4 {
			t.Fatalf("migrated catalog: title=%q tag=%q source=%q pages=%d posted=%d ordered=%d base=%d parent=%q indexes=%d: %v", title, tag, source, pages, posted, ordered, base, parent, indexes, err)
		}
		var violations int
		if err := migrated.QueryRow("SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil || violations != 0 {
			t.Fatalf("foreign key violations=%d: %v", violations, err)
		}
		if err := migrated.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
