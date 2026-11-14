# 02: Record displayed Panda galleries on the collector and flag them in catalog pages

What to build: Tana can tell the collector which Panda galleries a listing page displayed. The collector keeps one display record per gallery and record, with its time, and every catalog page marks each gallery as `displayed` or not. Nothing in the UI changes yet: ticket 03 sends the records and shows the flag.

This is the server side of spec behaviours 5 (durable state), 7, 12, 34 and 35 (display records), 36 (the `displayed` flag) and 37. Its layers are each specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0002_panda_gallery_displays.sql`: `panda_gallery_displays (gallery_id, displayed_at)`, primary key `(gallery_id, displayed_at)`, no foreign key, with a comment saying why (ADR 0019).
- `internal/collector/catalog`: `ErrInvalidDisplays` and `func (s *Service) RecordDisplays(ctx context.Context, ids []int64) error`. A record stores the whole page or nothing, and all its displays carry the collector database's time of the record. `List` sets `CatalogItem.Displayed` by looking up only the page's items. The page query itself doesn't change in this ticket.
- `internal/collectorapi`: `MaxCatalogDisplays = 100`, `CatalogItem.Displayed bool` (JSON `displayed`), and `func (c *Client) RecordCatalogDisplays(ctx context.Context, ids []int64) error`.
- `internal/collector/httpapi`: `HandleRecordCatalogDisplays`, registered as `POST /api/catalog/displays`. `catalogError` maps `ErrInvalidDisplays` to 400 `invalid_batch`.
- `internal/local/httpapi`: `HandleCollectorRecordCatalogDisplays`, registered as `POST /api/collector/catalog/displays`, in a new `catalog_seen.go`. It validates the list itself before calling the collector.
- Docs: a new `docs/adr/0019-collector-keeps-panda-seen-state.md`, in the style of the existing ADRs (title plus short prose). It says that the unseen condition must run inside the collector's page query (ADR 0018 keeps the databases separate) and test each candidate. So the collector now keeps the user's seen state for the Panda catalog as well as Panda metadata and archives (ADR 0004), and Tana reaches it only through the collector API. Display records and catch-up coverage are separate facts: each display keeps its time as evidence for later judgments, and a catch-up is one position that only advances. Neither references catalog rows, so the state survives every projection change, and identity is the gallery ID, so a new version is unseen. Add the four terms of the brief to `CONTEXT.md` after **Panda catalog**, as written: **Seen Panda gallery**, **Displayed Panda gallery**, **Unseen view**, **Catch-up**.

Test seams: package tests in `internal/collector/catalog` (a new `seen_test.go`, using `openCatalog`, `retain`, `projectFeed`), which may query `panda_gallery_displays` for times. The validation table in `internal/collector/httpapi/catalog_test.go`. A local test in `internal/local/httpapi/catalog_test.go` that runs the real collector handler behind `httptest.NewServer`, like `TestLocalCollectorSyncDownloadCategories`. Don't change existing assertions.

Blocked by: None

Status: resolved

- [x] `RecordDisplays` with IDs `[3, 1, 3]` succeeds and stores one display for gallery 1 and one for gallery 3. A later call for gallery 3 adds a second display with a later `displayed_at` and keeps the first.
- [x] `List` returns `displayed: true` for recorded galleries and `false` for the others, in the view of all galleries and with a search query.
- [x] A gallery recorded while it had provisional metadata (`projectFeed`) is still `displayed` after its collected metadata is projected. It is still `displayed` after its catalog row is removed and projected again.
- [x] After `pgtest.Reopen`, a recorded gallery is still `displayed`.
- [x] An empty list, more than 100 IDs, or an ID `<= 0` returns `ErrInvalidDisplays`. A list that mixes valid IDs with an ID `<= 0` stores none of them.
- [x] Collector `POST /api/catalog/displays` returns 204 for a valid list, 400 `invalid_batch` for an empty list, and 400 `invalid_json` for a body with unknown fields.
- [x] Through the real collector handler, local `POST /api/collector/catalog/displays` with `{"gallery_ids":[...]}` returns 204, and `GET /api/collector/catalog` then lists those galleries with `"displayed":true`. An empty list, 101 IDs, or a non-positive ID gives 400 `invalid_batch` without a collector request. A client with the wrong token gives 502 `collector_unauthorized`, and a nil client gives 503 `collector_not_configured`.
- [x] `docs/adr/0019-collector-keeps-panda-seen-state.md` exists with the content above, and `CONTEXT.md` defines the four terms.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Added migration 0002 `panda_gallery_displays`, `catalog.Service.RecordDisplays` and the `displayed` flag in `List` (`seen.go`), collector `POST /api/catalog/displays`, `collectorapi.RecordCatalogDisplays`, local `POST /api/collector/catalog/displays` (`catalog_seen.go`), ADR 0019 and four `CONTEXT.md` terms; the collector storage test now expects schema versions 1..N for the N embedded migrations.
