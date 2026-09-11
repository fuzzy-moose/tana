package feed

import (
	"database/sql"
	"log/slog"
	"testing"
	"time"
)

func TestFeedPromotesSitemapReferencesWithoutChangingTokensOrAttempts(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	if _, err := db.Exec(`INSERT INTO gallery_refs (gallery_id, token, metadata_priority, metadata_attempted_at, metadata_error)
		VALUES (1, 'original', 1, NULL, NULL), (2, 'original', 1, 123, 'unavailable'), (3, 'original', 1, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	capture(t, s.store, atomFeed("1/conflicting", "2/conflicting", "4/new"), time.Now())
	s.processPending(t.Context())
	for _, want := range []struct {
		id, priority int64
		token        string
		attempted    sql.NullInt64
		err          sql.NullString
	}{
		{id: 1, token: "original"},
		{id: 2, token: "original", attempted: sql.NullInt64{Int64: 123, Valid: true}, err: sql.NullString{String: "unavailable", Valid: true}},
		{id: 3, token: "original", priority: 1},
		{id: 4, token: "new"},
	} {
		var priority int64
		var token string
		var attempted sql.NullInt64
		var metadataError sql.NullString
		if err := db.QueryRow(`SELECT token, metadata_priority, metadata_attempted_at, metadata_error FROM gallery_refs WHERE gallery_id = ?`, want.id).
			Scan(&token, &priority, &attempted, &metadataError); err != nil {
			t.Fatal(err)
		}
		if token != want.token || priority != want.priority || attempted != want.attempted || metadataError != want.err {
			t.Fatalf("gallery %d: token=%q priority=%d attempted=%+v error=%+v", want.id, token, priority, attempted, metadataError)
		}
	}
}
