package feed

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/fuzzy-moose/tana/internal/collector/catalog"
	"github.com/fuzzy-moose/tana/internal/collectorapi"
)

func TestProvisionalDescriptionUsesCaptureOrderAndKeepsInitialDeadline(t *testing.T) {
	db := openTestDB(t, t.TempDir())
	s := &Service{store: newStore(db), logger: slog.New(slog.DiscardHandler)}
	browse := catalog.New(db, "https://panda.example.test")
	at := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	body := func(title, posted, tag string) []byte {
		return []byte(fmt.Sprintf(`<feed xmlns="http://www.w3.org/2005/Atom"><entry>
			<title>%s</title><link href="/g/1/token/"/><updated>%s</updated>
			<content type="xhtml"><div xmlns="http://www.w3.org/1999/xhtml"><p>Tags: artist:%s</p></div></content>
			</entry></feed>`, title, posted, tag))
	}
	older := capture(t, s.store, body("Older", at.Add(-time.Hour).Format(time.RFC3339), "old"), at)
	newer := capture(t, s.store, body("Newer", at.Format(time.RFC3339), "new"), at.Add(time.Hour))
	for _, id := range []int64{newer, older} {
		if err := s.process(t.Context(), id); err != nil {
			t.Fatal(err)
		}
	}
	result, err := browse.List(t.Context(), collectorapi.CatalogOptions{Query: "artist:new", PageSize: 24})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("latest captured tags: %+v, %v", result, err)
	}
	item := result.Items[0]
	if item.Title != "Newer" || item.PostedAt == nil || !item.PostedAt.Equal(at) || item.FirstSeenAt == nil || !item.FirstSeenAt.Equal(at) || item.FetchEligibleAt == nil || !item.FetchEligibleAt.Equal(at.Add(11*time.Hour)) {
		t.Fatalf("capture ordering/deadline: %+v", item)
	}
	lookup, err := browse.Lookup(t.Context(), 1)
	if err != nil || lookup.Metadata != nil {
		t.Fatalf("feed data became collected metadata: %+v, %v", lookup, err)
	}
	latest := capture(t, s.store, body("Latest", "invalid", "latest"), at.Add(2*time.Hour))
	if err := s.process(t.Context(), latest); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE gallery_refs SET metadata_error = 'upstream unavailable' WHERE gallery_id = 1`); err != nil {
		t.Fatal(err)
	}
	result, err = browse.List(t.Context(), collectorapi.CatalogOptions{PageSize: 24})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("failed collection lost feed entry: %+v, %v", result, err)
	}
	item = result.Items[0]
	if item.Title != "Latest" || item.PostedAt != nil || item.PageCount != nil || item.MetadataSource != "feed" || item.MetadataState != "failed" || !item.FetchEligibleAt.Equal(at.Add(11*time.Hour)) {
		t.Fatalf("replacement/deadline: %+v", item)
	}
	result, err = browse.List(t.Context(), collectorapi.CatalogOptions{Query: "artist:new", PageSize: 24})
	if err != nil || len(result.Items) != 0 {
		t.Fatalf("old provisional tags retained: %+v, %v", result, err)
	}
	if n := count(t, db, "SELECT count(*) FROM raw_feeds WHERE length(body) != 0"); n != 0 {
		t.Fatal("successful projection retained raw bodies")
	}
}
