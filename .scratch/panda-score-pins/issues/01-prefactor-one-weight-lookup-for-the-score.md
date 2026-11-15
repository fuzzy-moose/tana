# 01: Prefactor: One weight lookup for the score sum, the item weights and the tag strip

What to build: nothing the user sees changes. Today three places of `internal/collector/catalog` read the weights of a model with their own SQL: `scoreSum` and `markWeights` in `score_filter.go`, and `summarizeTags` in `catalog.go`. Tickets 04 and 07 change what a weight is (a pin, the author bonus), and a place that misses the change gives a card that contradicts the filter (spec **Fit**, item 1). From this ticket on, one private builder gives the SQL expression for a weight, and the three take every weight from it.

Its parts are specified in spec **Design → Collector: `internal/collector/catalog`**, **Ticket 01**:

- One private builder in `score_filter.go` returns the SQL expression for the calculated weight under the model whose ID a placeholder binds: of a tag, named by an expression for its `catalog_tags` ID, of the uploader and of the category of a gallery. The expression is NULL for a value without a weight.
- `scoreSum`, `markWeights` and `summarizeTags` take every weight from the builder. None of them names `panda_score_weights` itself.
- Behaviour stays as it is. `scoreSum` gives the same sum, with a scalar probe per tag (keep the reason that its comment gives). `markWeights` gives each scored item the values that have a weight, the strongest `scoreItemWeights`, in today's order. `summarizeTags` gives the same tags, weights and order.
- No exported name changes. `scoreTenths`, `belowThreshold`, `hidden`, `markScores`, `scoreFilter.recent` and `scoreFilter.heldOut` keep their behaviour.

Tickets 04 and 07 extend the builder: its weight then also depends on the pins and the author bonus that the score filter has read, and it names the source of a weight.

Test seam: the existing package tests of `internal/collector/catalog` pin all three readers (`TestAppliedModelHidesGalleriesBelowTheThreshold` for the score and the item weights, `summary_tags_test.go` for the strip, `score_recent_test.go` for the histograms). No test file changes, and no new test is needed.

Blocked by: None

Status: resolved

- [x] In the non-test code of `internal/collector/catalog`, only the new builder, `modelWeights` and `storeCandidate` name the table `panda_score_weights`.
- [x] The catalog page and the preview are as before: every existing test of `internal/collector/catalog`, `internal/collector/httpapi` and `internal/local/httpapi` passes.
- [x] The commits of this ticket change no `_test.go` file.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Added the private builder `weightSQL` in `internal/collector/catalog/score_filter.go`; `scoreSum`, `markWeights` and `summarizeTags` take every weight from it, with no test changes.
