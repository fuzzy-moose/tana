# 04: Display records say whether they are evidence

What to build: a record of displayed galleries carries the score filter state of the listing that showed them. The collector stores with each display whether it is evidence for the score model: a display in the "hidden only" state is not, and neither is a display from before verdicts were possible. The galleries are seen either way. The web sends the state in ticket 10, and the calculation reads the marker in ticket 05.

This is the server side of spec behaviours 5, 6 and 39, and behaviour 55. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0006_panda_display_evidence.sql`: the column and the one-time update of **Design → Collector schema**, with a comment that says what the column means and where the cutoff comes from.
- `internal/collector/catalog`: `RecordDisplays(ctx, ids, scoreFilter string)` (**Design → Display evidence**). `"hidden"` stores `evidence = false`. `""` and `"all"` store `true`. A value that `collectorapi.ValidScoreFilter` rejects returns `ErrInvalidDisplays`. `markDisplayed`, the unseen probe and `SetVerdict` stay as they are.
- `internal/collectorapi`: `RecordCatalogDisplays(ctx, ids, scoreFilter string)`, which sends `score_filter` only when it is not empty.
- `internal/collector/httpapi`: `HandleRecordCatalogDisplays` reads the optional `score_filter`.
- `internal/local/httpapi`: `HandleCollectorRecordCatalogDisplays` reads the optional `score_filter`, answers 400 `invalid_batch` itself for a value that `collectorapi.ValidScoreFilter` rejects, and forwards it.
- `CONTEXT.md`: the entry **Displayed Panda gallery** ends with "Each display keeps the time it was recorded and whether it is evidence for the Panda score model."

Two tickets come first. Ticket 02 gives this migration its number: it follows `0005_panda_score_overrides.sql`. Ticket 03 gives `collectorapi.ScoreFilterAll`, `ScoreFilterHidden` and `ValidScoreFilter`, which both services validate with here.

Existing callers and tests of `RecordDisplays` and `RecordCatalogDisplays` gain the argument `""`. Their assertions don't change.

Test seams: package tests in `internal/collector/catalog/seen_test.go` or a new file, which may read `panda_gallery_displays.evidence` directly. A migration test in `internal/collector/storage/storage_test.go` that starts from a database at version 5 that already holds displays. Hint: `pgtest.Open` takes any migration set, and `pgtest.Reopen` applies the full set to the same database. Cases in the existing validation table and in `TestLocalRecordCatalogDisplays`.

Blocked by: 02, 03

Status: resolved

- [x] `RecordDisplays(ids, "hidden")` stores displays with `evidence` false. `RecordDisplays(ids, "")` and `RecordDisplays(ids, "all")` store them with `evidence` true. Another state returns `ErrInvalidDisplays` and stores nothing.
- [x] A gallery recorded only with `"hidden"` has `Displayed` true in `List` and is absent from the unseen view.
- [x] The display that `SetVerdict` records has `evidence` true.
- [x] A database at version 5 holds one display older than `schema_migrations.applied_at` of version 4 and one newer. After the remaining migrations ran, the older display has `evidence` false and the newer one `evidence` true.
- [x] Collector `POST /api/catalog/displays` with `{"gallery_ids":[1],"score_filter":"hidden"}` returns 204, and with `"score_filter":"bogus"` 400 `invalid_batch`. A body without `score_filter` is accepted as before.
- [x] Through the real collector handler, local `POST /api/collector/catalog/displays` with `"score_filter":"hidden"` returns 204 and the stored displays have `evidence` false. `"score_filter":"bogus"` gives 400 `invalid_batch` without a collector request. A record without the field reaches the collector with the same body as before this ticket.
- [x] `CONTEXT.md` has the extended **Displayed Panda gallery** entry.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Migration 0006 adds `panda_gallery_displays.evidence` (false for displays before migration 4); `RecordDisplays` and `RecordCatalogDisplays` take the score filter state, which both HTTP handlers read and validate; `CONTEXT.md` entry extended.
