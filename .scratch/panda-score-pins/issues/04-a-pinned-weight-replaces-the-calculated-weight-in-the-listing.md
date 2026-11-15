# 04: A pinned weight replaces the calculated weight in the Panda listing

What to build: with an active Panda score model, a pin counts in the Panda gallery score in place of the calculated weight of its tag, uploader or category, also for a value that has no calculated weight, and a pin of 0 ignores the value. The next catalog page after a pin is saved or removed hides, shows, scores and ranks by it, with no calculation and no "Apply". Each summary tag and each score weight of a catalog item says when its weight is a pin. Without an active model, pins change nothing. The marks on the cards come in ticket 10.

This is spec behaviours 2 to 4, 6 to 9 and 23, the effect side of behaviours 5 and 11, the collector side of behaviours 24, 25, 27, 28 and 30, and the `source` of behaviour 54. Its layers are specified in spec **Design**:

- `internal/collectorapi`: the constant `ScoreSourcePin`, and `CatalogTag.Source` and `CatalogScoreWeight.Source` (`json:"source,omitempty"`).
- `internal/collector/catalog`: **Design → The weight rule** (without the favorite author part, which is ticket 07) and **Design → Pins in the listing (ticket 04)**. The builder of ticket 01 gives the effective weight and its source, and it probes `panda_score_pins` by key through the two unique indexes of migration 0008. `hidden`, `belowThreshold`, `markScores`, `markWeights`, `summarizeTags`, `scoreFilter.recent` and `hasHiddenUnseen` follow the builder. `belowThreshold` stays `false` without an active model, and `scoreFilter.inForce` does not look at pins.
- No route is added, and the handlers need no code: the new fields pass through `HandleCatalog` and `HandleCollectorCatalog` on the shared struct.

Hint: the score filter can read the pins once per `List`, with the reader of ticket 03, and leave the pin probe out of the expression while no pin exists.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `setVerdict`, `setOverride`, `scorePage`, `itemByID`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate`, `weightOf` and `assertSummaryTags` (its `summaryTag` helper may gain the source). One local test through the real collector handler in `internal/local/httpapi/catalog_test.go`, built like `TestLocalCatalogSummaryTagsRankWeightedTags`. Every view and score filter state runs the one expression of the builder, so the package tests pin it through `List` in the states they need and not once per view. Existing assertions stay unchanged.

Hints (optional): with `seedScoreEvidence`, `seedScoredGalleries` and `applyCandidate(t, s, -5)`, the calculated weights are about -0.69 for `female:glasses`, -0.47 for each of `artist:a` to `artist:f`, 0.69 for `male:hat` and 0.47 for the category Manga. The state `""` then lists galleries 102 and 103 and hides 101 (score about -0.69) and 104 (about -3.51). With `female:glasses` pinned to 5, gallery 104 scores about 2.18. A tag that only one retained gallery carries has no calculated weight.

Blocked by: 01, 03

Status: resolved

- [x] A pin replaces the calculated weight of its tag in the next catalog page, with no preview and no apply in between: a pin of 0 removes the tag's weight from the score, a pin above the calculated weight lifts a gallery over the threshold, a pin below it puts a gallery under the threshold, and removing the pin brings the calculated weight back.
- [x] A pin counts for a value without a calculated weight: a negative pin on a tag that the model has no weight for hides a gallery that was listed, with the rule `below_threshold` in the state `"hidden"`.
- [x] An uploader pin and a category pin count in the score of each gallery with that uploader or category.
- [x] `Score.Value` of an item is the sum of its effective weights. `Score.Weights` holds each value of the gallery that has an effective weight, also a pin of 0 and a pinned value without a calculated weight, strongest first, and a pinned value has the `Source` `"pin"`. A value with a calculated weight and no pin has an empty `Source`.
- [x] `SummaryTags` rank by the effective weight: a pinned tag has the pin as `Weight` and the `Source` `"pin"`, and a tag with a pin of 0 comes before a tag without a weight. A tag with a score override and a pin stays in the first group with its `Override`, and carries the pin as `Weight` with the `Source` `"pin"`.
- [x] The hiding rules keep their order: a pin, however large, does not hide a gallery with the verdict "Interesting" or with a "never hide" match, and does not show a gallery with an "always hide" match. The item's rule is then `never_hide` or `always_hide`.
- [x] Without an active model, pins have no effect: with pins stored and no override, `List` returns a nil `ScoreFilter` and lists every gallery, no item has a `Score`, and no summary tag has a `Weight` or a `Source`.
- [x] A pin outlives the model: after "Turn off" (`RemoveScoreModel`) and after a new candidate is applied, the pin counts in the score again.
- [x] Through the real collector handler, a local catalog page with an active model and one pinned tag has `"source":"pin"` on that tag's `summary_tags` entry and on its `score.weights` entry. A tag without a pin has no `source` key in either.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

The score filter reads the pins once per `List`, and its weight builder (`scoreFilter.weightSQL`, with `sourceSQL` and `pinSQL` in `score_filter.go`) gives each tag, uploader and category its pin before the model's weight, probing `panda_score_pins` only while a pin exists. `scoreSum`, `scoreTenths`, `markWeights` and `summarizeTags` take it from there, and `CatalogTag.Source` and `CatalogScoreWeight.Source` carry `"pin"`. Tests are in `score_pins_listing_test.go` and `TestLocalCatalogMarksPinnedWeights`.
