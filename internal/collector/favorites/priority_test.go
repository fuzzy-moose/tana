package favorites

import (
	"database/sql"
	"testing"
)

func TestFavoritesPromoteSitemapReferencesWithoutChangingTokensOrAttempts(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(`INSERT INTO gallery_refs (gallery_id, token, metadata_priority, metadata_attempted_at, metadata_error)
		VALUES (1, 'original', 1, NULL, NULL), (2, 'original', 1, 123, 'unavailable'), (4, 'original', 1, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	seed(t, s, fixtureEntries(1, 3, 1000))
	for _, want := range []struct {
		id, priority int64
		token        string
		attempted    sql.NullInt64
		err          sql.NullString
	}{
		{id: 1, token: "original"},
		{id: 2, token: "original", attempted: sql.NullInt64{Int64: 123, Valid: true}, err: sql.NullString{String: "unavailable", Valid: true}},
		{id: 3, token: "123456789a"},
		{id: 4, token: "original", priority: 1},
	} {
		var priority int64
		var token string
		var attempted sql.NullInt64
		var metadataError sql.NullString
		if err := s.db.QueryRow(`SELECT token, metadata_priority, metadata_attempted_at, metadata_error FROM gallery_refs WHERE gallery_id = ?`, want.id).
			Scan(&token, &priority, &attempted, &metadataError); err != nil {
			t.Fatal(err)
		}
		if token != want.token || priority != want.priority || attempted != want.attempted || metadataError != want.err {
			t.Fatalf("gallery %d: token=%q priority=%d attempted=%+v error=%+v", want.id, token, priority, attempted, metadataError)
		}
	}
}
