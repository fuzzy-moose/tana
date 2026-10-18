package feed

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFeedPromotesSitemapReferencesWithoutChangingTokensOrAttempts(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token, metadata_priority, metadata_attempted_at, metadata_error)
		VALUES (1, 'original', 1, NULL, NULL), (2, 'original', 1, to_timestamp(0.123), 'unavailable'), (3, 'original', 1, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	capture(t, s.store, atomFeed("1/conflicting", "2/conflicting", "4/new"), time.Now())
	s.processPending(t.Context())
	for _, want := range []struct {
		id, priority int64
		token        string
		attempted    pgtype.Timestamptz
		err          pgtype.Text
	}{
		{id: 1, token: "original"},
		{id: 2, token: "original", attempted: pgtype.Timestamptz{Time: time.UnixMilli(123), Valid: true}, err: pgtype.Text{String: "unavailable", Valid: true}},
		{id: 3, token: "original", priority: 1},
		{id: 4, token: "new"},
	} {
		var priority int64
		var token string
		var attempted pgtype.Timestamptz
		var metadataError pgtype.Text
		if err := db.QueryRow(t.Context(), `SELECT token, metadata_priority, metadata_attempted_at, metadata_error FROM gallery_refs WHERE gallery_id = $1`, want.id).
			Scan(&token, &priority, &attempted, &metadataError); err != nil {
			t.Fatal(err)
		}
		if token != want.token || priority != want.priority || attempted.Valid != want.attempted.Valid || !attempted.Time.Equal(want.attempted.Time) || metadataError != want.err {
			t.Fatalf("gallery %d: token=%q priority=%d attempted=%+v error=%+v", want.id, token, priority, attempted, metadataError)
		}
	}
}
