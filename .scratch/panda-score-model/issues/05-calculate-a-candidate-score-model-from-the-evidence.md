# 05: Calculate a candidate score model from the evidence

What to build: the collector can calculate a score model from the user's evidence and return it as a preview: the evidence counts, the weights of tags, uploaders and categories with their counts, a warning for few verdicts, and a reason when no model is possible. It stores the result as the one candidate, so that a later apply can name it. Nothing changes in any listing. Applying comes in ticket 06, the comparison with the active model in ticket 07, the error rates in ticket 08, the Tana route in ticket 09 and the page in ticket 11.

This is the server side of spec behaviours 1 to 4, 7 to 12, 14 (without the newer verdicts), 16 (weights and counts), 22, 23 and 50 (the candidate's ID). Behaviours 5 and 6 hold through the `evidence` column of ticket 04. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0007_panda_score_models.sql`: the two tables of **Design → Collector schema**.
- `internal/collectorapi`: `ScorePreviewOptions` and `ScorePreview` with the fields of ticket 05: `favorites`, `evidence` (`interesting`, `not_for_me`, `neutral`, `counted_favorites`, `favorites_synced_at`, `few_verdicts`), `candidate` (`id`, `calculated_at`, `favorites`, `weight_count`, `weights` with `kind`, `label`, `weight`, `interesting`, `not_for_me`, `neutral`), `no_model` (`no_verdicts`, `no_not_for_me`, `no_acceptable`), `overrides`. The JSON names are in **Design → `internal/collectorapi`**. No client method yet.
- `internal/collector/catalog`, new files: `PreviewScoreModel` and the pure weight functions with their constants (**Design → Calculation**). The constraints of that section hold: one transaction, the evidence read once, no candidate left behind but the new one, and no statement per weight when the weights are stored. In this ticket `options.Favorites == nil` means `false`.
- The preview validates `DefaultQuery` and `DefaultCategories` with ticket 01's builder and returns its errors. It doesn't use the default filter yet.
- `internal/collector/httpapi`: `POST /api/catalog/score/preview`, body `ScorePreviewOptions`, 200 with the preview.

The calculation reads only `panda_gallery_verdicts`, `panda_gallery_displays`, `favorites`, `favorite_categories`, `catalog_galleries`, `catalog_gallery_tags` and `catalog_tags`. That gives behaviour 11 by structure, without a test.

Test seams: plain table tests for the weight functions. Package tests on `pgtest` in a new test file, with `openCatalog`, `retain`, `projectFeed`, `setVerdict`, `RecordDisplays`, and direct SQL for favorites (see `TestCatalogFactsUseLatestCurrentFavoriteEvenWithoutMetadata` in `filters_test.go`) and for the `evidence` column. One case in `internal/collector/httpapi/catalog_test.go`.

Blocked by: 01, 02, 04

Status: resolved

- [x] The weight functions return no weight for a value that fewer than `MinSupport` evidence galleries carry, and no model when there is no "Not for me" evidence or no "Interesting" and no neutral evidence.
- [x] For a value carried only by "Not for me" galleries the weight is negative, for one carried only by "Interesting" galleries positive, and for one that the classes carry in the same proportions as all evidence it is 0 within 1e-9.
- [x] With the same proportions between the classes, a value with more evidence galleries gets a weight further from 0 than one with fewer, and 100 neutral galleries move a weight less than 100 "Interesting" galleries do.
- [x] `PreviewScoreModel` counts each gallery once: a gallery with three displays is one neutral gallery, and a gallery with a verdict and displays counts only under its verdict. A display with `evidence` false alone gives no neutral evidence. A verdict or display on a gallery without a catalog entry is not counted.
- [x] With `Favorites` true, a favorite that is in two favorite collections adds 1 to `counted_favorites` and to the "Interesting" counts of its tags. A favorite with a "Not for me" verdict counts as "Not for me" only. A displayed favorite without a verdict is a counted favorite, not neutral. With `Favorites` false, `counted_favorites` is 0 and that gallery is neutral.
- [x] `favorites_synced_at` is the newest `favorite_categories.synced_at`, and null when no collection was synced.
- [x] A tag that enough "Not for me" galleries carry appears in `weights` with a negative weight and its three counts, with the label `namespace:value`, the same label ticket 02's reader gives an override for that tag. Uploaders and categories appear with kind `uploader` and `category`. No weight has an empty label, also when evidence galleries have an empty uploader or category. A tag on fewer than `MinSupport` evidence galleries has no weight.
- [x] `weights` is ordered by absolute weight, largest first, and `weight_count` is the number of weights stored for the candidate in `panda_score_weights`.
- [x] A gallery with provisional metadata (`projectFeed`) and a verdict adds its feed tags to the counts.
- [x] Without any verdict, `candidate` is null and `no_model` is `no_verdicts`, also when `Favorites` is true and favorites exist. With "Interesting" verdicts and no "Not for me" verdict, `no_model` is `no_not_for_me`. With "Not for me" verdicts, no "Interesting" verdict and no neutral gallery, `no_model` is `no_acceptable`. In all three cases no model row is stored.
- [x] `few_verdicts` is true while either verdict count is under `FewVerdicts`.
- [x] After two previews, `panda_score_models` holds exactly one row, with the second preview's `id`.
- [x] `overrides` lists the overrides of ticket 02 with `id`, `kind`, `label` and `rule`, and is an empty JSON array without any.
- [x] A preview changes no catalog page: `List` returns the same items before and after it.
- [x] No query of the calculation names a table outside the seven listed above (checked by reading the queries; no test).
- [x] Collector `POST /api/catalog/score/preview` with `{"favorites":false,"default_query":"","default_categories":[]}` returns 200 with `evidence`, `overrides` and `candidate` or `no_model`. An unknown default category gives 400 `invalid_category`, an invalid default query 400 `invalid_query`, and a body with unknown fields 400 `invalid_json`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Migration `0007_panda_score_models.sql`, `collectorapi.ScorePreviewOptions`/`ScorePreview` with the `no_model` constants, `catalog.PreviewScoreModel` (`score_model.go`) over the pure weight functions and constants in `score_weights.go`, and collector `POST /api/catalog/score/preview`; covered by `score_weights_test.go`, `score_model_test.go` and `TestScorePreviewValidation`. Previews take a `SHARE ROW EXCLUSIVE` lock on `panda_score_models` before replacing the candidate, so concurrent previews leave one.
