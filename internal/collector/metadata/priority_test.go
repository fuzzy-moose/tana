package metadata

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/fuzzy-moose/tana/internal/panda"
)

func TestExplicitFetchesThenFreshDiscoveriesPrecedeSitemapBackfill(t *testing.T) {
	for _, rediscovered := range []bool{false, true} {
		t.Run(fmt.Sprintf("rediscovered=%v", rediscovered), func(t *testing.T) {
			db := openDB(t, t.TempDir())
			seedRefs(t, db, 1, 2, 100, 101)
			if rediscovered {
				seedRefs(t, db, 102, 103, 104)
			}
			if _, err := db.Exec(`UPDATE gallery_refs SET metadata_priority = 1 WHERE gallery_id IN (1, 2, 102, 103, 104)`); err != nil {
				t.Fatal(err)
			}
			var batches [][]int64
			s := batchService(t, db, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				ids := requestedIDs(t, req)
				batches = append(batches, ids)
				entries := make([]panda.Metadata, len(ids))
				for i, id := range ids {
					entries[i] = panda.Metadata{ID: id, Token: fmt.Sprintf("token%d", id)}
					if id == 2 {
						entries[i].ParentID, entries[i].ParentToken = 102, "token102"
						entries[i].CurrentID, entries[i].CurrentToken = 103, "token103"
						entries[i].FirstID, entries[i].FirstToken = 104, "token104"
					}
				}
				return metadataResponse(t, entries...), nil
			}))
			fetchJob(t, s, panda.GalleryRef{ID: 2, Token: "token2"})
			collectBatch(t, s)
			collectBatch(t, s)
			if want := [][]int64{{2}, {100, 101, 102, 103, 104, 1}}; !reflect.DeepEqual(batches, want) {
				t.Fatalf("metadata batches = %v, want %v", batches, want)
			}
		})
	}
}
