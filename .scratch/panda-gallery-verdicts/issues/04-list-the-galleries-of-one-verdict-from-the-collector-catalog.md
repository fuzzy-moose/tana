# 04: List the galleries of one verdict from the collector catalog

What to build: the catalog API can list only the galleries with one verdict, "Interesting" or "Not for me", in catalog order and combined with the search and every filter. Nothing in the UI changes yet: ticket 05 adds the two views to the toolbar.

This is the server side of spec behaviours 20, 22, 23, 24 (the rejected combination), 25 (paging), and 41 (the `verdict` parameter). Its layers are each specified in spec **Design**:

- `internal/collectorapi`: `CatalogOptions.Verdict string`. `Catalog` sends `verdict=<value>` on every request, empty included.
- `internal/collector/catalog`: `List` with `options.Verdict`. An unknown value, and a verdict together with `Unseen`, return `ErrInvalidQuery`. The page comes from a verdict page query in place of `catalogPageQuery`, with the same shared predicate and the same result columns, so the rest of `List` runs unchanged. Paging follows the rules of the view of all galleries.
- **The verdict page query must start from the verdicts**, as spec **Design → Collector: `internal/collector/catalog`** describes: a `MATERIALIZED` common table expression joins `panda_gallery_verdicts`, filtered to the verdict, to `catalog_galleries g` by primary key and applies the predicate, with no `ORDER BY` and no `LIMIT` inside. The query around it sorts and limits. Never add the verdict as a condition to `catalogPageQuery`: that scans the catalog in order to find a few rows.
- `internal/collector/httpapi`: `HandleCatalog` passes `verdict` to `options.Verdict` unparsed.
- `internal/local/httpapi`: `HandleCollectorCatalog` passes `verdict` to `options.Verdict` unparsed. The collector's 400 `invalid_query` already passes through `writeCatalogCollectorError`.

Test seams: package tests in `internal/collector/catalog/verdicts_test.go` (with `openCatalog`, `retain`, `assertCatalogIDs`). The table in `TestCatalogPaginationAndQueryValidation` for the parameter (add rows; don't change existing ones). One local test in `internal/local/httpapi/catalog_test.go` through the real collector handler. The query plan can't be judged on test data: it is a manual check of the spec.

Blocked by: 02

Status: resolved

- [x] `List` with `Verdict: "interesting"` returns exactly the galleries with that verdict, newest catalog position first, each with `Verdict` `"interesting"` and `Displayed` true. `Verdict: "not_for_me"` returns exactly the others with a verdict. Galleries without a verdict are in neither.
- [x] A verdict view combined with a search query, with one category, with several categories, and with a default query and default categories returns only the galleries that have the verdict and pass every filter.
- [x] An expunged gallery with the verdict is listed only with `IncludeExpunged`.
- [x] With more galleries than the page size, following `NextCursor` and then `PreviousCursor` walks the verdict view forward and back without skipping or repeating a gallery, the first page has no `PreviousCursor`, and the last page has no `NextCursor`.
- [x] After a verdict is changed or removed, the gallery is absent from the view of its old verdict on the next `List`.
- [x] A verdict for a gallery ID without a catalog row is not listed and doesn't fail the page.
- [x] `Verdict: "maybe"` returns `ErrInvalidQuery`, and so does a valid verdict with `Unseen: true`. An empty `Verdict` lists as before.
- [x] Collector `GET /api/catalog?verdict=interesting` returns 200. `?verdict=maybe` and `?verdict=interesting&unseen=true` give 400 `invalid_query`.
- [x] Through the real collector handler, local `GET /api/collector/catalog?verdict=not_for_me` lists only the galleries with that verdict, and `?verdict=maybe` gives 400 `invalid_query`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`CatalogOptions.Verdict` is sent by `collectorapi.Client.Catalog` and passed through by both catalog handlers; `catalog.List` validates it (unknown value or with `Unseen` gives `ErrInvalidQuery`) and pages through `verdictPageQuery` in `verdicts.go`, a materialized join from `panda_gallery_verdicts` to `catalog_galleries` with the sort and limit outside. Tests in `internal/collector/catalog/verdicts_test.go`, rows in `TestCatalogPaginationAndQueryValidation`, and the verdict view in `TestLocalCatalogVerdicts`.
