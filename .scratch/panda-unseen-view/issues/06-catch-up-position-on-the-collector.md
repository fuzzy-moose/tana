# 06: Keep a catch-up position on the collector and apply it to the unseen view

What to build: Tana can ask the collector to catch up at a gallery. The collector stores that gallery's catalog position, as the one catch-up position, and from then on the unseen view leaves out every gallery at or before it. That includes galleries that a filter hides and galleries that join the catalog later with an older position. Nothing in the UI changes yet: ticket 07 adds the action.

This is the server side of spec behaviours 21–24, 28 (no display records), 34 and 35 (catch-up part) and 38. Its layers are each specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0003_panda_catalog_catch_up.sql`: the singleton `panda_catalog_catch_up (id, ordered_at, gallery_id, caught_up_at)`, no foreign key. No row means no catch-up.
- `internal/collector/catalog`:
  - `ErrGalleryNotInCatalog`, and `func (s *Service) CatchUp(ctx context.Context, galleryID int64) error`. It takes the gallery's current `(ordered_at, gallery_id)` from `catalog_galleries` and stores it, with the time of this catch-up, only when it is greater than the stored position. Catch-ups that run at the same time only ever advance the position.
  - `List` with `Unseen` reads the catch-up position in its transaction before building the query. When one exists, it limits candidates to positions after it, as a keyset bound on `(ordered_at, gallery_id)` with bound arguments.
- `internal/collectorapi`: `func (c *Client) CatalogCatchUp(ctx context.Context, galleryID int64) error`, posting `{"gallery_id": n}` to `/api/catalog/catch-up`. 204 returns nil. Other statuses return `downloadResponseError`.
- `internal/collector/httpapi`: `HandleCatalogCatchUp`, registered as `POST /api/catalog/catch-up`. `catalogError` maps `ErrGalleryNotInCatalog` to 404 `gallery_not_found`. `ErrInvalidGalleryReference` already maps to 400 `invalid_gallery_reference`.
- `internal/local/httpapi`: `HandleCollectorCatalogCatchUp` in `catalog_seen.go`, registered as `POST /api/collector/catalog/catch-up`. It validates `gallery_id > 0` itself (400 `invalid_gallery_reference`). `writeCatalogCollectorError` gains upstream 404 `gallery_not_found` → 404 `gallery_not_found`.

Test seams: package tests in `internal/collector/catalog/seen_test.go`. The validation table in `internal/collector/httpapi/catalog_test.go`. `internal/local/httpapi/catalog_test.go` with the real collector handler behind `httptest.NewServer`, as in ticket 02. Don't change existing assertions.

Blocked by: 04

Status: resolved

- [x] After `CatchUp` at gallery G, the unseen view leaves out every gallery whose `(ordered_at, gallery_id)` is at or before G's, including G. Galleries newer than G are still listed unless displayed.
- [x] The coverage also applies under a query, categories, a default filter and `IncludeExpunged`. A gallery that the query hid at catch-up time and that is older than G stays out of the unseen view without that query.
- [x] A later `CatchUp` at a gallery older than G leaves the unseen view unchanged. One at a newer gallery extends the coverage to it.
- [x] `CatchUp` calls at an older and a newer gallery that run at the same time leave the newer gallery's position stored, whichever finishes first.
- [x] After the catch-up at G, `panda_catalog_catch_up` holds G's `ordered_at` and ID and the time of that catch-up. A catch-up that doesn't advance the position leaves the row unchanged, time included.
- [x] A gallery projected after the catch-up whose position is at or before G's, whether provisional or collected, is not in the unseen view.
- [x] Galleries covered only by a catch-up are `displayed: false` in the view of all galleries, and the catch-up adds no rows to `panda_gallery_displays`.
- [x] The coverage survives removal and re-projection of G's catalog row.
- [x] `CatchUp(0)` returns `ErrInvalidGalleryReference`, and an ID without a catalog row returns `ErrGalleryNotInCatalog`. Neither changes the coverage.
- [x] Collector `POST /api/catalog/catch-up` returns 204 for a catalog gallery, 404 `gallery_not_found` for an unknown one, 400 `invalid_gallery_reference` for `0`, and 400 `invalid_json` for unknown fields.
- [x] Through the real collector handler, local `POST /api/collector/catalog/catch-up` returns 204, and the unseen view then leaves out the covered galleries. It gives 404 `gallery_not_found` for an unknown gallery, 400 `invalid_gallery_reference` for `gallery_id <= 0` without a collector request, 502 `collector_unauthorized` with a wrong token, and 503 `collector_not_configured` with a nil client.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Migration `0003_panda_catalog_catch_up.sql`; `catalog.Service.CatchUp` (single conditional upsert that only advances) and the catch-up keyset bound in unseen `List`; `collectorapi.Client.CatalogCatchUp`; collector `POST /api/catalog/catch-up` and local `POST /api/collector/catalog/catch-up` with `gallery_not_found` mapping; tests in `catalog/seen_test.go` and both `httpapi/catalog_test.go` files.
