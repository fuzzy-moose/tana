# 01: Prefactor: one predicate builder for catalog queries

What to build: nothing the user can see. `catalog.Service.List` builds its predicate inline today. The score preview needs the same predicate without paging, to judge the listing with the default filter applied (tickets 05, 07 and 08). This ticket moves that part of `List` into one private function, so those tickets call it instead of copying it.

This is spec **Fit** item 1 and **Design → Collector: `internal/collector/catalog` → Ticket 01**:

- In `internal/collector/catalog/catalog.go`, a private function takes the transaction and the `collectorapi.CatalogOptions`, and returns the predicate on alias `g`, the bound `pgdb.Args`, and the page categories for `catalogPageQuery`. It covers what `List` does today from `searchNamespaces` to the `IncludeExpunged` condition: the search predicate, the default-filter predicate, the `tagMatcher` error, both category selections with the choice of the smaller one as page categories, and `g.expunged IS DISTINCT FROM true`.
- `List` calls it and then adds the unseen conditions and the cursor condition, and chooses `verdictPageQuery` or `catalogPageQuery`, exactly as today.
- No exported name, no SQL text of the page query, and no error value changes.

Keep the behaviour unchanged, leave every existing test file unedited, and add no test. The package tests observe `List` under every search, filter, category and paging case, so they pin this ticket. Tickets 07 and 08 pin the builder's use with only a default filter, through the results of the preview.

Blocked by: None

Status: resolved

- [x] `go test ./internal/collector/catalog/` passes, and the commit changes no `_test.go` file.
- [x] `List` gets its search, default-filter, category and expunged predicate, its arguments and its page categories from one private function in `internal/collector/catalog`, and builds none of them inline (checked by reading `List`; no test).
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Moved the search, default-filter, category and expunged predicate of `catalog.Service.List` into the private `catalogPredicate` in `internal/collector/catalog/catalog.go`, which returns the predicate, its `pgdb.Args` and the page categories; `List` calls it and adds the unseen and cursor conditions as before.
