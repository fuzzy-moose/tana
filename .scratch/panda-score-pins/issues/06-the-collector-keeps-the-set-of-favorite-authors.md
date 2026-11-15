# 06: The collector keeps the set of favorite authors

What to build: the collector knows the favorite authors: each `artist` tag that at least 2 Panda favorites with catalog tags carry. It keeps them in a table with their favorite counts and brings the table up to date whenever a Panda favorite sync completes. A completed sync adds authors and updates counts, and only a completed full re-sync removes an author. A running or failed sync changes nothing. The update that brings this feature fills the table from the stored favorites. Nothing reads the set yet: the bonus comes in ticket 07 and the evaluation page in ticket 13.

This is spec behaviours 12 to 17. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0009_favorite_authors.sql`: the table `favorite_authors` of **Design → Collector schema**, with a comment on its purpose, and one statement that fills it from the stored favorites by the definition below.
- `internal/collector/favorites`: **Design → Collector: `internal/collector/favorites` (ticket 06)**. New queries in `queries.sql` and regenerated `dbgen`. `store.savePage` runs them when the sync is done, in its transaction, after `ReconcileFavorites` and before any return of the completed branch (the baseline branch returns early): one statement adds the favorite authors and sets their favorite counts, and for a full sync (`current.Full`) one more removes every row that is no longer a favorite author. No other code writes the table, and no lock is needed (spec **Fit**, item 9).
- The definition: a tag of the namespace `artist` that at least 2 distinct `gallery_id` values of `favorites` carry in `catalog_gallery_tags`. Favorites of every stored collection count, and a gallery in several collections counts one time. A favorite without rows in `catalog_gallery_tags` gives no author. The minimum 2 and the namespace are fixed values of the queries and of the migration.
- Docs:
  - `CONTEXT.md`, a new entry after **Pinned weight**, in the file's layout: **Favorite author**: "An `artist` tag carried by at least two Panda favorites whose tags the Panda catalog has, as of the last completed Panda favorite sync. A gallery in several Panda favorite collections counts once."
  - `docs/adr/0019-collector-keeps-panda-seen-state.md`, in the paragraph on pinned weights or after it: the collector keeps the set of favorite authors in a table of its own, read from its favorites and the catalog's tags in the transaction that completes a Panda favorite sync, so that the catalog page query probes one row per tag and never counts favorites per candidate. Only a completed full re-sync removes an author.

Migration 0009 follows migration 0008 of ticket 03: collector migrations are numbered without a gap, which is why this ticket waits for ticket 03.

Test seams: package tests in `internal/collector/favorites` with `testStore`, `seed`, `admit`, `syncEntries`, `manualService`, `drain`, `fixtureEntries` and `pageFunc`, which may read `favorite_authors` directly. The migration's fill is tested in `internal/collector/storage/storage_test.go` with the migrations up to 0008 in an `fstest.MapFS` and `pgtest.Reopen`, like `TestDisplayEvidenceMarksDisplaysFromBeforeVerdicts`.

Hint (optional): a favorite gets catalog tags with `catalog.Project` in a transaction, after its `gallery_refs` row exists, as `internal/local/httpapi/catalog_test.go` does. `internal/collector/catalog` does not import `favorites`, so the test import makes no cycle.

Blocked by: 03

Status: resolved

- [x] After a completed sync, `favorite_authors` holds each `artist` tag that at least 2 favorites carry in the catalog, with the number of those favorites. A tag that one favorite carries gives no row.
- [x] A gallery that is a favorite of several collections counts one time, and favorites of a collection of another Panda account count like the others.
- [x] Tags of other namespaces give no row, also `group` tags that 2 favorites carry.
- [x] A favorite counts only with catalog tags: a favorite without a catalog entry adds nothing to an author's count. Once its gallery is in the catalog with the tag, the next completed sync counts it, also a sync that is not full.
- [x] A completed sync that is not full adds a new favorite author and raises a count, and it removes no author, also not one whose favorites no longer carry the tag in the catalog. The next completed full re-sync removes that author.
- [x] A sync that is running or has failed changes nothing in the table, also when it has saved new favorites.
- [x] A completed full re-sync removes an author whose favorites are gone upstream so that fewer than 2 remain, and it keeps an author whose 2 favorites are in another collection.
- [x] The migration fills the table from the favorites stored before it, by the same definition.
- [x] `CONTEXT.md` has the **Favorite author** entry, and ADR 0019 names the set of favorite authors.
- [x] `go generate ./internal/local/storage/ ./internal/collector/storage/` leaves no uncommitted change. `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Migration `0009_favorite_authors.sql` creates and fills `favorite_authors`; `favorites.store.savePage` runs the new `SaveFavoriteAuthors` (and, for a full sync, `RemoveFormerFavoriteAuthors`) queries when a sync completes; tests in `internal/collector/favorites/favorite_authors_test.go` and `internal/collector/storage/storage_test.go`; `CONTEXT.md` and ADR 0019 name the favorite authors.
