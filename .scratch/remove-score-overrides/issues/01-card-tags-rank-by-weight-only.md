# 01: Card tags rank by weight only

What to build: the tags that a catalog page of the collector offers to a card are ranked by their effective weight alone. A score override gives a tag no rank and no rule: a pinned tag carries its pin's weight and source, whatever else it has. Spec behaviour 11 (the rank); shape in spec **Design → Catalog page**.

Only the summary tags change here. Hiding and the item's score follow in ticket 02, and the overrides' storage and API stay until tickets 06 and 07. Tests of behaviour that stays are seeded with a model and pins instead of an override (spec **Fit** 2). `TestLocalCatalogScoreFilter` (`internal/local/httpapi`) loses its assertion on the first tag's `override`.

Blocked by: None

Status: resolved

- [x] With an active model, summary tags are ranked by the absolute value of their effective weight, the strongest first, then the tags without a weight in namespace order, cut at the summary tag limit.
- [x] A pinned tag carries its pinned weight and the source `pin` and ranks by that weight, also with a pin of 0, which ranks before the tags without a weight.
- [x] A favorite author's tag is marked as one while a model is active, whatever else the tag carries.
- [x] `collectorapi.CatalogTag` has no override field, and the build passes.
- [x] `go vet ./...` and `go test ./...` pass, and `gofmt -l .` prints nothing.

## Resolution

`summarizeTags` ranks only with an active model and by `tagRank` (absolute effective weight) alone; `overrideRank` and `collectorapi.CatalogTag.Override` are gone. Override-ranking tests became weight-ranking tests (`summary_tags_test.go`, `score_pins_listing_test.go`), and `TestLocalCatalogScoreFilter` lost its tag override assertion.
