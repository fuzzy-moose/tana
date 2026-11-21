# 10: A display gives only seen state

What to build: going to the next page records the page's galleries as seen and nothing more. The record no longer says in which score filter state the listing was, and the collector no longer stores whether a display is evidence. Every display is recorded alike, in "Filter on", "Filter off" and "Hidden only".

This is spec behaviour 8 (record side). The calculation stopped reading displays in ticket 04. Fixed in spec **Design → Routes**, **`internal/collectorapi`**, **Collector schema** and **Web (Display record)**. The body changes on all three sides in this one ticket, because `server.DecodeJSON` rejects unknown fields: a browser that still sent `score_filter` would get 400, and its pages would never become seen.

- Migration, next free number (0015): drops `panda_gallery_displays.evidence`.
- `internal/collector/catalog/seen.go`: `func (s *Service) RecordDisplays(ctx context.Context, ids []int64) error`.
- `internal/collector/httpapi`, `internal/local/httpapi`: the request body is `{"gallery_ids": [...]}`.
- `internal/collectorapi/catalog_displays.go`: `func (c *Client) RecordCatalogDisplays(ctx context.Context, ids []int64) error`.
- `web/src/panda/api.ts`, `PandaCatalog.tsx`: `recordPandaDisplays(galleryIDs: number[])`.

Tests of removed behaviour are deleted, not ported, and no new test pins that something is gone (spec **Design → Test seams**): `display_evidence_test.go`, `TestDisplayEvidenceMarksDisplaysFromBeforeVerdicts` (it reads the dropped column after a full migration), the `score_filter` cases of the two display validation tests, and the web test that expects `score_filter` in the record go.

Test seams: `catalog.Service.RecordDisplays` and the seen tests in `internal/collector/catalog`; `TestRecordCatalogDisplaysValidation`; `TestLocalRecordCatalogDisplays`, which captures the body sent to the collector; a migration test in `internal/collector/storage`; `web/src/panda/PandaScoreFilter.test.tsx`.

Blocked by: 04

Status: resolved

- [x] A database with display rows from before the migration migrates: every display row is still there, and its galleries are still marked `displayed` and absent from the unseen view.
- [x] The collector answers `POST /api/catalog/displays` with 204 for `{"gallery_ids":[1,2]}` and marks both galleries displayed.
- [x] Tana answers `POST /api/collector/catalog/displays` with 204 for `{"gallery_ids":[1,2]}` and sends the collector a body with the one key `gallery_ids`.
- [x] An empty list, more than 100 IDs and an ID that is not positive are still refused with 400 `invalid_batch` on both sides.
- [x] In the listing, Next posts exactly `{"gallery_ids":[5,3]}` for a page with galleries 5 and 3 in each of the states "Filter on", "Filter off" and "Hidden only".
- [x] Removal: the column `panda_gallery_displays.evidence` (dropped by the new migration), the `scoreFilter` parameter of `Service.RecordDisplays`, of `Client.RecordCatalogDisplays` and of `recordPandaDisplays`, and the `score_filter` field of both request bodies no longer exist. A search for "evidence" finds nothing in `internal/collector/catalog/seen.go`.
- [x] The `go` and `frontend` checks of the green check pass, and `gofmt -l .` prints nothing.

## Resolution

Migration 0015 drops `panda_gallery_displays.evidence`; `catalog.Service.RecordDisplays`, `collectorapi.Client.RecordCatalogDisplays` and `recordPandaDisplays` take only gallery IDs, and both display routes accept only `{"gallery_ids": [...]}`. The display evidence tests went; a migration test, the collector validation test and the score filter web test cover the new record.
