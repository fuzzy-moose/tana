package favorites

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestFavoritesPromoteSitemapReferencesWithoutChangingTokensOrAttempts(t *testing.T) {
	s := testStore(t)
	if _, err := s.db.Exec(t.Context(), `INSERT INTO gallery_refs (gallery_id, token, metadata_priority, metadata_attempted_at, metadata_error)
		VALUES (1, 'original', 1, NULL, NULL), (2, 'original', 1, to_timestamp(0.123), 'unavailable'), (4, 'original', 1, NULL, NULL)`); err != nil {
		t.Fatal(err)
	}
	seed(t, s, fixtureEntries(1, 3, 1000))
	for _, want := range []struct {
		id, priority int64
		token        string
		attempted    pgtype.Timestamptz
		err          pgtype.Text
	}{
		{id: 1, token: "original"},
		{id: 2, token: "original", attempted: pgtype.Timestamptz{Time: time.UnixMilli(123), Valid: true}, err: pgtype.Text{String: "unavailable", Valid: true}},
		{id: 3, token: "123456789a"},
		{id: 4, token: "original", priority: 1},
	} {
		var priority int64
		var token string
		var attempted pgtype.Timestamptz
		var metadataError pgtype.Text
		if err := s.db.QueryRow(t.Context(), `SELECT token, metadata_priority, metadata_attempted_at, metadata_error FROM gallery_refs WHERE gallery_id = $1`, want.id).
			Scan(&token, &priority, &attempted, &metadataError); err != nil {
			t.Fatal(err)
		}
		if token != want.token || priority != want.priority || attempted.Valid != want.attempted.Valid || !attempted.Time.Equal(want.attempted.Time) || metadataError != want.err {
			t.Fatalf("gallery %d: token=%q priority=%d attempted=%+v error=%+v", want.id, token, priority, attempted, metadataError)
		}
	}
}
