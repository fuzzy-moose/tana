# 02: The catalog page ranks a card's tags by weight

What to build: with an active Panda score model, a catalog page from the collector offers each card the gallery's tags ranked by the absolute value of their weight, the strongest first, followed by the tags without a weight in today's order. Each weighted tag carries its weight. The cards of the Panda listing then show the strongest tags first, still all gray. The colours come in ticket 03, and the override tags in ticket 04.

This is spec behaviours 1 to 5, 7, 8 and 21, the collector side of behaviours 6, 11, 17 and 20, and the Go side of behaviour 22. Its layers are specified in spec **Design**:

- `internal/collectorapi/catalog.go`: the type `CatalogTag` with `Namespace`, `Value`, `Term` and `Weight` (no `Override` yet), and `CatalogItem.SummaryTags []CatalogTag`.
- `internal/collector/catalog`: **Design → `internal/collector/catalog`**, with the second and third group only: `List` passes the score filter to the summary lookup, which ranks all tags of each of the page's galleries before it keeps the first `gallerysearch.SummaryTagLimit`. The constraints of that section hold: one statement over the page's items, today's order taken from the database, the weight probed by `tag_id` in the active model, and no change to `markScores` and `markWeights`. While the score filter is not in force, the result is exactly today's.
- `internal/gallerysearch/search.go`: the comment on `SummaryTagLimit` states only the limit. The comment on the collector's summary lookup states its selection.
- No route is added, and the handlers need no code: the new field passes through `HandleCatalog` and `HandleCollectorCatalog` on the shared struct. The local gallery list (`internal/local/gallery`) needs no change.

Two existing tests change, and no other existing assertion does:

- `TestCatalogProjectionUpdatesAndRawTagCompletions` (`catalog_test.go`): the literal becomes `[]collectorapi.CatalogTag` with the same values.
- `TestAppliedModelHidesGalleriesBelowTheThreshold` (`score_apply_test.go`): its last loop requires that gallery 104's summary tags hold no `female` tag. That is the selection this ticket replaces. The second criterion below takes its place.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `projectFeed`, `setVerdict`, `scorePage`, `itemByID`, `unseenPage`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate` and `weightOf`. One local test through the real collector handler in `internal/local/httpapi/catalog_test.go`, built like `TestLocalCatalogScoreFilter`. Hint for that test: a model has a weight for a tag only when at least three evidence galleries carry it, and it needs both sides of the evidence, for example three galleries with `female:glasses` and the verdict "Not for me" and three with `male:hat` and the verdict "Interesting", then `PreviewScoreModel` and `ApplyScoreModel`.

In the criteria, the weights are those of `seedScoreEvidence`: `female:glasses` about -0.69, each of `artist:a` to `artist:f` about -0.47, `male:hat` about 0.69.

Blocked by: None

Status: resolved

- [x] Without an active model and without overrides, `List` gives every item the same `SummaryTags` as before this ticket: its first six tags by namespace, then value, each with a nil `Weight`.
- [x] With the model of `seedScoreEvidence` applied, gallery 104 (`female:glasses` and the six artist tags) has the `SummaryTags` `female:glasses`, `artist:a`, `artist:b`, `artist:c`, `artist:d`, `artist:e`, in this order, and no `artist:f`. Each has a `Weight` equal to the candidate's weight for that tag.
- [x] With that model, a gallery with the tags `artist:a`, `artist:u1` and `male:hat` has the `SummaryTags` `male:hat`, `artist:a`, `artist:u1`: the stronger positive tag comes before the weaker negative one, and the tag without a weight comes last with a nil `Weight`.
- [x] With that model, a gallery with `artist:u1` to `artist:u6` and `male:hat` has the `SummaryTags` `male:hat` with its weight, then `artist:u1` to `artist:u5` with a nil `Weight`. A gallery with only tags without a weight has its first six tags in today's order, each with a nil `Weight`.
- [x] With that model, a gallery with the uploader "Bad One", the category Manga and the tags `artist:u1` and `male:hat` has exactly the `SummaryTags` `male:hat`, `artist:u1`: no entry stands for the uploader or the category.
- [x] Gallery 104 has the same ranked `SummaryTags` in the filter states `"all"` and `"hidden"`. A gallery with those tags and a "Not for me" verdict has them in the "Not for me" view, and a gallery that the filter does not hide has its ranked `SummaryTags` in the state `""` and in the unseen view.
- [x] A gallery projected from a feed entry (`projectFeed`) with the tags `artist:u1` and `male:hat` has the `SummaryTags` `male:hat`, `artist:u1`. After its collected metadata with the tags `artist:u2` and `female:glasses` is projected, its `SummaryTags` are `female:glasses`, `artist:u2`.
- [x] After `SetScoreThreshold`, the next `List` gives gallery 104 the same `SummaryTags` as before. After `RemoveScoreModel`, the next `List` gives it `artist:a` to `artist:f`, each with a nil `Weight`.
- [x] `Score.Value` and `Score.Weights` of every item are as before this ticket: the other assertions of `TestAppliedModelHidesGalleriesBelowTheThreshold` pass unchanged.
- [x] Through the real collector handler, a local `GET /api/collector/catalog` page without a filter in force has `summary_tags` entries with exactly the keys `namespace`, `value` and `term`. A gallery without tags has `"summary_tags": []`, never `null`, with and without an active model. With an active model, the first `summary_tags` entry of a gallery with a weighted tag and a tag without a weight is the weighted tag, with a number `weight`, although namespace order puts the other tag first, and the other entry has no `weight` key.
- [x] The page of the local gallery list is as before: the existing tests of `internal/local/gallery` and `internal/local/httpapi` pass unchanged.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged except the two named above.

## Resolution

Added `collectorapi.CatalogTag` (with `Weight`) as the type of `CatalogItem.SummaryTags`; `summarizeTags` in `internal/collector/catalog/catalog.go` takes the score filter and, while it is in force, probes each tag's weight in the active model and stable-sorts all tags by absolute weight before keeping the first six. Tests in `summary_tags_test.go`, the replaced assertion in `score_apply_test.go`, and `TestLocalCatalogSummaryTagsRankWeightedTags` in `internal/local/httpapi/catalog_test.go`.
