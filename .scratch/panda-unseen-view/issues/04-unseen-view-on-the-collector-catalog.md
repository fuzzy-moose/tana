# 04: Serve the unseen view from the collector catalog

What to build: a catalog request can ask for the unseen view. The collector then leaves out every displayed gallery, keeps the order and every other filter, and pages without a way back. Nothing in the UI changes yet: ticket 05 adds the view control.

This is the server side of spec behaviours 3, 4, 13, 15–18 and 36 (`unseen`). Its layers are each specified in spec **Design**:

- `internal/collectorapi`: `CatalogOptions.Unseen bool`. `Catalog` sends `unseen=<bool>` on every request, like `include_expunged`.
- `internal/collector/catalog`, `List` with `options.Unseen`:
  - A `before` cursor returns `ErrInvalidPagination`.
  - The unseen condition (the gallery has no display) goes into the shared predicate, so each multi-category `UNION ALL` branch carries it too. It must stay a correlated probe per candidate, like the `-tag` check in `tagMatcher.match`. Never collect display IDs into an array or a `NOT IN` list. The catalog holds millions of galleries.
  - `NextCursor` is set from the last item whenever the page has items, even without more results. `PreviousCursor` is never set. An empty page returns neither cursor: the reversed, inclusive recovery for emptied pages doesn't apply.
  - Without `Unseen`, `List` behaves exactly as today.
- `internal/collector/httpapi`: `HandleCatalog` parses `unseen` with `strconv.ParseBool`. An error gives 400 `invalid_query`.
- `internal/local/httpapi`: `HandleCollectorCatalog` parses `unseen` the same way and forwards it.

Test seams: package tests in `internal/collector/catalog/seen_test.go`, recording displays with `RecordDisplays` from ticket 02. The validation table in `internal/collector/httpapi/catalog_test.go`. `internal/local/httpapi/catalog_test.go` for parsing and forwarding (an upstream stub that checks the query, as `TestPandaCatalogThroughLocalProxy` does, is enough). Don't change existing assertions.

Blocked by: 02

Status: resolved

- [x] With `Unseen`, `List` returns the same galleries as without it, minus the displayed ones, in the same order (`ordered_at`, then gallery ID, newest first).
- [x] That holds with a `Query`, one category, two categories (the `UNION ALL` path), a `DefaultQuery` and `DefaultCategories`, and with and without `IncludeExpunged`.
- [x] After recording only the galleries of a page filtered by a query, the unseen view without that query still lists the galleries the query hid.
- [x] Recording gallery 1 doesn't remove gallery 2 from the unseen view. A new version of a gallery has its own ID.
- [x] Every non-empty unseen page has `next_cursor`, including the last page, and no unseen page has `previous_cursor`.
- [x] Once every gallery after an earlier page's `next_cursor` has been recorded, an unseen request with that cursor returns an empty page with neither cursor and no error. While unseen galleries remain after that position, the same request returns them.
- [x] A gallery retained after the first unseen page was read, and newer than that page, is missing from the page at that page's `next_cursor` but listed on the unseen page without a cursor.
- [x] An unseen request with a `before` cursor returns `ErrInvalidPagination`.
- [x] Collector `GET /api/catalog?unseen=maybe` and local `GET /api/collector/catalog?unseen=garbage` give 400 `invalid_query`. Local `GET /api/collector/catalog?unseen=true` sends `unseen=true` to the collector, and a request without it sends `unseen=false`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

`CatalogOptions.Unseen` is sent on every catalog request and parsed by the collector and local handlers; `catalog.List` with it rejects `before` cursors, adds a correlated `NOT EXISTS` probe on `panda_gallery_displays` to the shared predicate, and returns a next cursor on every non-empty page and never a previous one. Tests in `seen_test.go` and the collector and local `catalog_test.go`.
