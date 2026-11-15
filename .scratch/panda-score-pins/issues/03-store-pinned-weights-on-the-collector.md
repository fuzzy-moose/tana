# 03: Store pinned weights on the collector

What to build: Tana can tell the collector to pin the weight of one tag, uploader or Panda gallery category to a number, to replace that number, and to remove the pin. The collector keeps the pins as one shared state that belongs to no model. A pin does not change a score yet (ticket 04), and nothing in the UI changes (tickets 11 and 14).

This is spec behaviours 1, 10, 51 and 55 (pins), and the stored side of behaviours 5 and 11. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0008_panda_score_pins.sql`: the table of **Design → Collector schema**, with a comment that says why it references `catalog_tags` but no model and no gallery row.
- `internal/collectorapi/catalog_score.go`: `ScorePin`, `Client.SetScorePin`, `Client.RemoveScorePin`.
- `internal/collector/catalog`, new file: `ErrInvalidScorePin`, `maxScoreWeight` (**Design → Limits**), `SetScorePin`, `RemoveScorePin` and the private reader of all pins (**Design → Pins (ticket 03)**). The value rules of `SetScoreOverride` move into one private helper that both use (spec **Fit**, item 7). `SetScoreOverride` keeps its behaviour and its errors, and its tests pass unedited. `SetScorePin` takes no lock on `panda_score_models`.
- `internal/collector/httpapi`: `POST /api/catalog/score/pins` (200 with the stored pin) and `DELETE /api/catalog/score/pins/{id}` (204), and the case `ErrInvalidScorePin` → 400 `invalid_pin` in `catalogError`. A body without `weight`, or with `"weight": null`, is an invalid pin.
- `internal/local/httpapi/catalog_score.go`: `HandleCollectorSetScorePin` and `HandleCollectorRemoveScorePin`, registered as `POST /api/collector/catalog/score/pins` and `DELETE /api/collector/catalog/score/pins/{id}`. They follow the two override handlers. `writeCatalogCollectorError` passes 400 `invalid_pin` through.
- Docs:
  - `CONTEXT.md`, the entry **Panda gallery score** becomes: "The sum of the weights of a gallery's tags, uploader, and category under the active Panda score model. A pinned weight replaces the model's weight of its value."
  - `CONTEXT.md`, a new entry after **Score override**, in the file's layout (term line, then the definition): **Pinned weight**: "A weight that the user sets by hand for one tag, uploader, or Panda gallery category, also for a value that has no weight in the model. It belongs to no Panda score model and stays until the user removes it. A pinned weight of 0 ignores the value."
  - `docs/adr/0019-collector-keeps-panda-seen-state.md`: one more paragraph. It says that the collector keeps the pinned weights with the score overrides, because both are the user's judgments and the score filter reads them for each candidate inside the catalog page query. A pinned weight names its tag by the tag vocabulary, or keeps its uploader or category as text, and references no model and no gallery row, so it survives every projection change, every calculation and every change of the active model. Describe the design as it is, without "now" or "also added".

Test seams: package tests in `internal/collector/catalog` (new test file, with `openCatalog`, `retain` and `setOverride`), which may read `panda_score_pins` directly. A validation table in `internal/collector/httpapi/catalog_test.go`, like `TestScoreOverrideValidation`. A local test in `internal/local/httpapi/catalog_test.go` that runs the real collector handler behind `httptest.NewServer`, like `TestLocalScoreOverrides`. That a pin outlives a model is pinned in ticket 04, where it shows in a score.

Blocked by: None

Status: resolved

- [x] `SetScorePin` for a tag stores one row that references the tag's `catalog_tags` row, found without regard to case and with `_` for a space. It returns the stored pin: its ID, the kind `tag`, the label as `namespace:value` in the tag's stored spelling, and the weight. The private reader returns the same pin.
- [x] An uploader pin stores the trimmed name, and a category pin stores the category as `panda.NormalizeCategories` gives it. A weight of 0 and a negative weight far below every calculated weight are stored as given.
- [x] `SetScorePin` returns `ErrTagNotFound` for a tag that no gallery ever had. It returns `ErrInvalidScorePin` for a tag value without a colon or with an empty part, a blank value of any kind, an unknown kind, an unknown category, and a weight that is NaN, infinite or above 1e300 in magnitude. None of these stores a row.
- [x] A second `SetScorePin` for the same tag, uploader or category leaves one row with the same ID and the new weight.
- [x] `RemoveScorePin` deletes the row with that ID, succeeds for an ID that has no row, and returns `ErrInvalidScorePin` for an ID `<= 0`.
- [x] A value can have a score override and a pin: both rows exist after both are set for the same tag.
- [x] Collector `POST /api/catalog/score/pins` returns 200 with the stored pin as `{"id","kind","label","weight"}`. A body without `weight` and an unknown kind give 400 `invalid_pin`, an unknown tag 404 `tag_not_found`, a body with unknown fields 400 `invalid_json`. `DELETE /api/catalog/score/pins/{id}` returns 204, and an ID that is not a positive integer gives 400 `invalid_pin`.
- [x] Through the real collector handler, local `POST /api/collector/catalog/score/pins` returns 200 with the pin, and local `DELETE /api/collector/catalog/score/pins/{id}` returns 204 and removes it. 400 `invalid_pin` and 404 `tag_not_found` pass through. An ID that is not a positive integer gives 400 `invalid_pin` without a collector request. Without a collector both routes answer 503 `collector_not_configured`, and with a rejected token 502 `collector_unauthorized`.
- [x] The existing tests of `score_overrides_test.go` pass unedited.
- [x] `CONTEXT.md` has the new **Pinned weight** entry and the changed **Panda gallery score** entry, and ADR 0019 has the paragraph on pinned weights.
- [x] `go generate ./internal/local/storage/ ./internal/collector/storage/` leaves no uncommitted change. `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Migration 0008 adds `panda_score_pins`; `catalog.SetScorePin`, `RemoveScorePin` and the reader `scorePins` live in `score_pins.go`, sharing `parseScoreTarget` with `SetScoreOverride`; collector and local pin routes, `collectorapi.ScorePin` with its client methods, `invalid_pin` mapping, and the CONTEXT.md and ADR 0019 entries were added. TestLocalScorePins covers the local 400 `invalid_pin` for a missing or null weight.
