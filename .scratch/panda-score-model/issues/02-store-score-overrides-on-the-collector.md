# 02: Store score overrides on the collector

What to build: Tana can tell the collector to always hide or never hide the galleries of one tag, uploader or Panda gallery category, and to remove such a rule. The collector keeps the overrides as one shared state. Nothing hides yet (ticket 03), and nothing in the UI changes (ticket 13).

This is the server side of spec behaviours 25, 26, 48 and 53 (overrides), and behaviours 56 and 59. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0005_panda_score_overrides.sql`: the table of **Design → Collector schema**, with a comment that says why it references `catalog_tags` but no gallery row.
- `internal/collectorapi`: the kind and rule constants, `ScoreOverride`, `Client.SetScoreOverride`, `Client.RemoveScoreOverride` (the lines marked ticket 02).
- `internal/collector/catalog`, new file: `ErrInvalidScoreOverride`, `ErrTagNotFound`, `SetScoreOverride`, `RemoveScoreOverride`, and the private reader of all overrides with their labels (**Design → Overrides**).
- `internal/collector/httpapi`: `POST /api/catalog/score/overrides` and `DELETE /api/catalog/score/overrides/{id}`, and the two new cases in `catalogError`.
- `internal/local/httpapi`, new file for the score routes: `HandleCollectorSetScoreOverride` and `HandleCollectorRemoveScoreOverride`, registered as `POST /api/collector/catalog/score/overrides` and `DELETE /api/collector/catalog/score/overrides/{id}`. `writeCatalogCollectorError` passes 400 `invalid_override` and 404 `tag_not_found` through.
- Docs:
  - `CONTEXT.md`, after **Catch-up**, in the file's layout (term line, then the definition):
    - **Panda score model**: A set of weights for tags, uploaders, and Panda gallery categories, calculated from the user's evidence (Panda gallery verdicts, displayed Panda galleries, and optionally Panda favorites), together with a score threshold. At most one model is active.
    - **Panda gallery score**: The sum of the weights of a gallery's tags, uploader, and category in the active Panda score model.
    - **Score threshold**: The score below which the score filter hides a gallery.
    - **Score override**: A user rule, "always hide" or "never hide", for one tag, uploader, or Panda gallery category.
    - **Score filter**: The hiding of Panda galleries in the catalog listing by the active Panda score model, the score overrides, and the user's own verdicts. It is in force while a model is active or an override exists.
  - `docs/adr/0019-collector-keeps-panda-seen-state.md`: one more paragraph. It says that the collector keeps the Panda score model, its threshold and the score overrides with the seen state and the verdicts, because the score filter tests each candidate inside the catalog page query and the model is calculated from the verdicts, display records and favorites the collector holds. Tana sends its default filter with each preview request, because the collector does not keep it. Weights and tag overrides reference the tag vocabulary, which projections only extend, and no gallery row, so they survive every projection change. The collector stores no score per gallery. Describe the design as it is, without "now" or "also added".

Test seams: package tests in `internal/collector/catalog` (new test file, with `openCatalog` and `retain`), which may read `panda_score_overrides` directly. A validation table in `internal/collector/httpapi/catalog_test.go`, like `TestCatalogVerdictValidation`. A local test in `internal/local/httpapi/catalog_test.go` that runs the real collector handler behind `httptest.NewServer`, like `TestLocalCatalogVerdicts`.

Blocked by: None

Status: resolved

- [x] `SetScoreOverride("tag", "Female:Big_Hat", "always_hide")` stores one tag row that references the `catalog_tags` row of `female:big hat`, when a retained gallery carries that tag. The private reader returns it with kind `tag`, rule `always_hide` and the label `female:big hat` in the tag's stored spelling.
- [x] `SetScoreOverride` with a tag no gallery ever had returns `ErrTagNotFound`. A tag value without a colon, with an empty namespace or an empty value, an empty or blank value of any kind, an unknown kind, an unknown rule, and a category outside the ten Panda gallery categories each return `ErrInvalidScoreOverride`. None of them stores a row.
- [x] `SetScoreOverride("uploader", " Some One ", "never_hide")` stores the value `Some One`. `SetScoreOverride("category", "Artist CG", "always_hide")` stores `artist cg`.
- [x] A second `SetScoreOverride` for the same tag, uploader or category with the other rule leaves one row with the new rule.
- [x] `RemoveScoreOverride` deletes the row with that ID, succeeds for an ID that has no row, and returns `ErrInvalidScoreOverride` for an ID `<= 0`.
- [x] An override survives the re-projection of every gallery that carries its tag (`retain` the gallery again with other tags): the row and its label stay.
- [x] Collector `POST /api/catalog/score/overrides` with a valid body returns 204. An invalid override gives 400 `invalid_override`, an unknown tag 404 `tag_not_found`, a body with unknown fields 400 `invalid_json`. `DELETE /api/catalog/score/overrides/3` returns 204, and `/overrides/0` and `/overrides/x` give 400 `invalid_override`.
- [x] Through the real collector handler, local `POST /api/collector/catalog/score/overrides` returns 204 and the row exists in the collector database, an unknown tag gives 404 `tag_not_found`, an invalid override 400 `invalid_override`, and local `DELETE /api/collector/catalog/score/overrides/{id}` returns 204 and removes the row. A non-positive or non-numeric ID gives 400 `invalid_override` without a collector request. A client with the wrong token gives 502 `collector_unauthorized`, and a nil client 503 `collector_not_configured`.
- [x] `CONTEXT.md` defines the five terms, and ADR 0019 has the score paragraph.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Migration `0005_panda_score_overrides.sql`; `collectorapi/catalog_score.go` (constants, `ScoreOverride`, client methods); `catalog/score_overrides.go` (`SetScoreOverride`, `RemoveScoreOverride`, private `scoreOverrides` reader); collector routes and `catalogError` cases; local `catalog_score.go` handlers, routes and error pass-through; CONTEXT.md terms and the ADR 0019 paragraph.
