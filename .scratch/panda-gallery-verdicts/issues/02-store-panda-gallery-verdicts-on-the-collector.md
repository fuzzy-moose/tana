# 02: Store Panda gallery verdicts on the collector and return them with catalog pages

What to build: Tana can tell the collector the user's verdict on a Panda gallery, change it, and remove it. The collector keeps one verdict per gallery with its time, makes the gallery displayed when a verdict is given, and every catalog page carries each gallery's verdict. Nothing in the UI changes yet: ticket 03 adds the controls.

This is the server side of spec behaviours 13 (the display), 17, 18, 31 and 36–40, and behaviours 41 (the `verdict` on items), 42 and 43. Its layers are each specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0004_panda_gallery_verdicts.sql`: the table of spec **Design → Collector schema**, with a comment saying why it has no foreign key (ADR 0019), in the style of `0002_panda_gallery_displays.sql`.
- `internal/collectorapi`: `VerdictInteresting`, `VerdictNotForMe`, `ValidVerdict`, `CatalogItem.Verdict` (JSON `verdict,omitempty`), `SetCatalogVerdict`, `RemoveCatalogVerdict`.
- `internal/collector/catalog`, new file `verdicts.go`: `ErrInvalidVerdict`, `SetVerdict`, `RemoveVerdict`. `SetVerdict` stores the verdict and one display for the gallery atomically. `List` sets `CatalogItem.Verdict` by looking up only the page's items, next to `markDisplayed`. The page query doesn't change in this ticket.
- `internal/collector/httpapi`: `HandleSetCatalogVerdict` and `HandleRemoveCatalogVerdict`, registered as `PUT` and `DELETE /api/catalog/verdicts/{id}`. `catalogError` maps `ErrInvalidVerdict` to 400 `invalid_verdict`. An `{id}` that is not an integer is an invalid gallery reference.
- `internal/local/httpapi`, new file `catalog_verdicts.go`: `HandleCollectorSetCatalogVerdict` and `HandleCollectorRemoveCatalogVerdict`, registered as `PUT` and `DELETE /api/collector/catalog/verdicts/{id}`. They validate the ID and the verdict themselves before calling the collector.
- Docs:
  - `CONTEXT.md`: add after **Displayed Panda gallery**: "**Panda gallery verdict**: The user's judgment of one Panda gallery, "Not for me" or "Interesting". A displayed Panda gallery without a verdict is neutral." Use the file's layout (term line, then the definition). Extend the definition of **Displayed Panda gallery** so it also covers a gallery the user gave a verdict: "A Panda gallery shown on a listing page that the user left by going to the next page, or one the user gave a verdict. Each display keeps the time it was recorded."
  - `docs/adr/0019-collector-keeps-panda-seen-state.md`: add one paragraph. It says that the collector also keeps the user's verdicts on Panda galleries, because they belong with the seen state: a verdict makes its gallery displayed, and the verdict views are collector page queries. A verdict is one row per gallery ID with its time and no copy of the metadata, so a later evaluation reads the metadata current at that time. Like the seen state it references no catalog row, survives every projection change, and does not carry over to a new version of the gallery. Describe the design as it is, without "now" or "also added".

Test seams: package tests in `internal/collector/catalog` (a new `verdicts_test.go`, using `openCatalog`, `retain`, `projectFeed`, `displayedIDs` and `unseenPage` from the existing test files), which may query `panda_gallery_verdicts` for `given_at`. A validation table in `internal/collector/httpapi/catalog_test.go`, like `TestCatalogCatchUpValidation`. A local test in `internal/local/httpapi/catalog_test.go` that runs the real collector handler behind `httptest.NewServer`, like `TestLocalCatalogCatchUp`. Don't change existing assertions.

Three guards get no test, because the structure gives them: `CatchUp`, `RecordDisplays` and the projection don't touch `panda_gallery_verdicts` (behaviour 18), the table is keyed by gallery ID and has no foreign key (behaviour 40), and its columns are the three of the migration (behaviour 37). Keep it that way.

Blocked by: None

Status: resolved

- [x] After `SetVerdict(id, "interesting")`, `List` returns that gallery with `Verdict` `"interesting"` and `Displayed` true, in the view of all galleries and with a search query. Galleries without a verdict have an empty `Verdict`, and their JSON has no `verdict` field.
- [x] After `SetVerdict` on a gallery that was unseen, the unseen view no longer lists it, and `panda_gallery_displays` holds a display for it.
- [x] `SetVerdict` with the other verdict replaces the stored one and moves `given_at` later. `SetVerdict` with the verdict the gallery already has keeps `given_at`. The table never holds two rows for one gallery.
- [x] After `RemoveVerdict`, `List` returns the gallery without a verdict and still `Displayed`, and the unseen view still leaves it out. `RemoveVerdict` on a gallery without a verdict succeeds.
- [x] A verdict given while the gallery had provisional metadata (`projectFeed`) is still returned after its collected metadata is projected, after its catalog row is removed and projected again, and after its metadata is projected as expunged (listed with `IncludeExpunged`).
- [x] `SetVerdict` with an ID `<= 0` returns `ErrInvalidGalleryReference`, and with a verdict other than the two returns `ErrInvalidVerdict`. Neither stores a verdict or a display. `RemoveVerdict` with an ID `<= 0` returns `ErrInvalidGalleryReference`.
- [x] Collector `PUT /api/catalog/verdicts/7` with `{"verdict":"interesting"}` returns 204. `{"verdict":"maybe"}` gives 400 `invalid_verdict`, a body with unknown fields 400 `invalid_json`, and `/api/catalog/verdicts/0` and `/api/catalog/verdicts/x` 400 `invalid_gallery_reference`. `DELETE /api/catalog/verdicts/7` returns 204.
- [x] Through the real collector handler, local `PUT /api/collector/catalog/verdicts/{id}` with `{"verdict":"not_for_me"}` returns 204, and `GET /api/collector/catalog` then lists that gallery with `"verdict":"not_for_me"` and `"displayed":true`. A `PUT` with `"interesting"` changes the listed verdict. `DELETE` returns 204, and the gallery is then listed without `verdict` and with `"displayed":true`.
- [x] Local `PUT` with another verdict gives 400 `invalid_verdict`, and `PUT` or `DELETE` with a non-positive or non-numeric ID gives 400 `invalid_gallery_reference`, each without a collector request. A client with the wrong token gives 502 `collector_unauthorized`, and a nil client gives 503 `collector_not_configured`.
- [x] `CONTEXT.md` defines **Panda gallery verdict** and the extended **Displayed Panda gallery**, and ADR 0019 has the verdict paragraph.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Added migration 0004 (`panda_gallery_verdicts`), `collectorapi` verdict constants, `CatalogItem.Verdict` and client calls, `catalog.SetVerdict`/`RemoveVerdict` with the page verdict lookup in `List`, collector and local `PUT`/`DELETE` verdict routes, and the CONTEXT.md and ADR 0019 docs, with tests in `verdicts_test.go` and both `catalog_test.go` files.
