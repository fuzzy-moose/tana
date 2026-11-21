# 09: The collector keeps no verdicts

What to build: the update deletes every stored verdict, and the collector can no longer store, list or serve one. The collector has no verdict routes and no verdict views, a catalog item carries no verdict, and nothing but a display record or a catch-up makes a gallery seen. Galleries that had a verdict stay seen, because their display records stay. A catalog request that still carries `verdict` is answered as without it. ADR 0019 describes the design as it now is.

This is spec behaviours 2, 4, 5 and 7 (collector side). Fixed in spec **Design → Routes**, **`internal/collectorapi`**, **Collector schema** and **ADR 0019**:

- Migration, next free number (0014): drops `panda_gallery_verdicts`. No export. Earlier migrations stay as they are.
- `internal/collector/catalog`: `verdicts.go` goes (`SetVerdict`, `RemoveVerdict`, `ErrInvalidVerdict`, the marking of items and the verdict page query); `List` no longer knows a verdict view.
- `internal/collector/httpapi`: the two verdict routes, their handlers and the error code `invalid_verdict` go; `HandleCatalog` no longer reads `verdict`.
- `internal/collectorapi`: `catalog_verdicts.go` goes; `CatalogOptions.Verdict`, `CatalogItem.Verdict` and the `verdict` parameter of `Client.Catalog` go.
- `docs/adr/0019-collector-keeps-panda-seen-state.md`: the correction the brief orders.

Tests of removed behaviour are deleted, not ported, and no new test pins that something is gone (spec **Design → Test seams**): `verdicts_test.go`, `TestCatalogVerdictValidation`, the verdict-view case of `TestScoreFilterLeavesVerdictViewsAndRejectsInvalidStates`, and every remaining `setVerdict` call and verdict-view listing in the tests go. Tests of the rule order and of the summary tags keep their override and score cases, and `display_evidence_test.go` loses its verdict case (the file itself goes in ticket 10).

Test seams: a migration test (`pgtest.Open` on the migrations before this one, `pgtest.Reopen` on all) followed by a listing through `catalog.New`, in the package that can do both; `internal/collector/httpapi` through `NewHandler` for the ignored `verdict` parameter.

Blocked by: 01, 02, 04

Status: resolved

- [x] A database with verdict rows and the display rows recorded with them migrates, and every display row is still there.
- [x] After that migration, a gallery that had the verdict "Not for me" is on the catalog page without a model and without overrides, it is marked `displayed`, and it is absent from the unseen view.
- [x] `GET /api/catalog?verdict=not_for_me` answers 200 with the same items as the request without the parameter, also together with `unseen=true` and with `score_filter=hidden`.
- [x] Removal: the collector's route table has no route under `/api/catalog/verdicts`; `internal/collector/catalog/verdicts.go`, `internal/collectorapi/catalog_verdicts.go`, `CatalogOptions.Verdict`, `CatalogItem.Verdict` (the `verdict` key of a catalog item) and the error code `invalid_verdict` no longer exist; the new migration drops `panda_gallery_verdicts`.
- [x] Removal: a search for "verdict" in any letter case finds nothing under `internal/` except the files under `internal/collector/storage/` (migrations and their tests) and the tests of the ignored `verdict` query parameter in the two `httpapi` packages.
- [x] `docs/adr/0019-collector-keeps-panda-seen-state.md` does not contain "verdict", says that the model is calculated from the Panda favorites and the catalog that the collector holds, says that Tana sends its default filter with each preview request, and no longer calls a display evidence. It describes only the current design.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Migration `0014_drop_panda_gallery_verdicts.sql` drops the verdicts (pinned by `TestDropVerdictsMigrationKeepsDisplays` in `internal/collector/storage`); `catalog/verdicts.go`, `collectorapi/catalog_verdicts.go`, `CatalogOptions.Verdict`, `CatalogItem.Verdict`, the collector's verdict routes and handlers and `invalid_verdict` are gone, so `List` has no verdict view and `HandleCatalog` ignores `verdict` (`TestCatalogIgnoresVerdictParameter` through `NewHandler`); verdict tests and calls were deleted, and ADR 0019 describes the favorites-based model and displays without evidence.
