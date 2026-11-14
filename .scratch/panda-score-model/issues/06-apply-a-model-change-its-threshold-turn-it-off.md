# 06: Apply a model, change its threshold, turn it off

What to build: the collector can make the candidate the active model with a threshold, change the threshold of the active model, and remove it. With an active model, a catalog page hides the galleries whose score is below the threshold (rule 4), reports the threshold, and gives each gallery its score and strongest weights. The Tana routes come in ticket 09 and the UI in tickets 10 and 12.

This is the server side of spec behaviours 19, 20, 21, 31 (rule 4), 32, 33, 41, 47, 49, 50 and 53, and the collector part of behaviour 58. Its layers are specified in spec **Design**:

- `internal/collectorapi`: `CatalogScoreFilter.ThresholdTenths`, `CatalogItemScore.Value` and `Weights`, `CatalogScoreWeight` (the lines marked ticket 06).
- `internal/collector/catalog`:
  - `ErrScoreCandidateNotFound`, `ErrInvalidScoreThreshold`, `ErrNoActiveScoreModel`, `ApplyScoreModel`, `SetScoreThreshold`, `RemoveScoreModel` (**Design → Apply, threshold, turn off**).
  - `ApplyScoreModel` follows the locking of that section (ADR 0018): its transaction locks `panda_score_models` in `SHARE ROW EXCLUSIVE` mode first, then checks the candidate, and only then deletes the active model and promotes the candidate. It never deletes the active model for a candidate that is gone. `SetScoreThreshold` and `RemoveScoreModel` are one statement each.
  - `List` reads the active model's ID and threshold with the overrides. The filter is in force when a model is active or an override exists. The hidden condition gains rule 4 as its last case: `floor(score * 10) < threshold_tenths`. The constraints of **Design → The score filter in `List`** hold: one SQL score expression for a gallery and a model ID serves the page condition and the per-card lookup (ticket 07 uses it a third time), and the page condition reaches the active model's weights by key, from the candidate's own tags, uploader and category.
  - The per-card lookup sets `Value`, up to five `Weights` (strongest by absolute weight, from all tags of the gallery, its uploader and its category, each with kind and label), and `Rule` `"below_threshold"` when rules 2 and 3 don't match and the score is below the threshold. `ScoreFilter.ThresholdTenths` is set with an active model.
- `internal/collector/httpapi`: `POST /api/catalog/score/apply`, `PUT /api/catalog/score/threshold`, `DELETE /api/catalog/score/model`, and the three new cases in `catalogError`.

Test seams: package tests on `pgtest` in `internal/collector/catalog`, building a candidate with `PreviewScoreModel` from seeded verdicts. Hint: a helper that seeds enough "Not for me" galleries with one tag and enough "Interesting" galleries with another keeps the cases short, and tickets 07 and 08 can reuse it. For the two cases with concurrent calls, `TestConcurrentCatchUpsKeepTheNewestPosition` and `waitForLockWaiters` in `seen_test.go` show how a test holds calls at a lock, and a second candidate can be inserted with SQL. A validation table in `internal/collector/httpapi/catalog_test.go`.

Blocked by: 03, 05

Status: resolved

- [x] After `ApplyScoreModel(candidate.ID, t)`, `List` with `ScoreFilter` `""` leaves out the galleries without a verdict whose `floor(score * 10)` is below `t`, `"hidden"` lists them, and `"all"` lists every gallery. The result's `ScoreFilter.ThresholdTenths` is `t`.
- [x] Each item then has `Score.Value` equal to the sum of the active weights of all its tags, its uploader and its category, and `Score.Weights` with at most five entries ordered by absolute weight. A gallery with more than six tags, whose strongest weight belongs to a tag outside its `SummaryTags`, has that tag in `Score.Weights` and in `Score.Value`.
- [x] A hidden item listed by `"all"` or `"hidden"` has `Score.Rule` `"below_threshold"`. A gallery below the threshold that a "never hide" override matches is listed by `""` with `Score.Rule` `"never_hide"`. One with an "Interesting" verdict is listed by `""`. A gallery above the threshold with a "Not for me" verdict is left out by `""`.
- [x] `ApplyScoreModel` with an ID that is not a stored candidate, and with the ID of a candidate that a later `PreviewScoreModel` replaced, returns `ErrScoreCandidateNotFound`. The active model, its threshold and the listing are as before.
- [x] While another transaction holds an uncommitted deletion of the candidate, as a running preview does, `ApplyScoreModel` for that candidate does not return. After that transaction commits, it returns `ErrScoreCandidateNotFound`, and the active model, its threshold and its weights are as before.
- [x] Two `ApplyScoreModel` calls that run at the same time, for two different stored candidates, both return nil, and exactly one model is active afterwards.
- [x] Applying a second candidate leaves one active model, the second, and no weight rows of the first.
- [x] A `PreviewScoreModel` after an apply leaves the active model and the listing unchanged, and its candidate can be applied in turn.
- [x] `SetScoreThreshold` changes which galleries `""` leaves out without changing a weight. It returns `ErrNoActiveScoreModel` without an active model. `ApplyScoreModel` and `SetScoreThreshold` return `ErrInvalidScoreThreshold` below -1000 and above 1000.
- [x] After `RemoveScoreModel`, no gallery is hidden by its score, an existing "always hide" override still hides, and with no override left a gallery with a "Not for me" verdict is listed by `""` again and the result has a nil `ScoreFilter`. `RemoveScoreModel` without an active model succeeds.
- [x] A gallery with provisional metadata whose feed tags put it below the threshold is hidden. After collected metadata with other tags is projected, `""` lists it.
- [x] A gallery hidden by its score is absent from the unseen view and from the pages recorded there. After the threshold is lowered below its score, the unseen view lists it.
- [x] A verdict view lists the same galleries with and without an active model, and its items carry `Score.Value`.
- [x] Collector `POST /api/catalog/score/apply` with a stored candidate returns 204, with an unknown `candidate_id` 409 `score_candidate_not_found`, with `"threshold_tenths":1001` 400 `invalid_threshold`. `PUT /api/catalog/score/threshold` returns 204 with an active model and 409 `score_model_not_active` without. `DELETE /api/catalog/score/model` returns 204. Bodies with unknown fields give 400 `invalid_json`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`catalog.ApplyScoreModel`, `SetScoreThreshold` and `RemoveScoreModel` (`score_apply.go`, apply under a `SHARE ROW EXCLUSIVE` table lock); `List` reads the active model and adds rule 4 through one `scoreTenths` SQL expression, and the per-card lookup sets `Value`, five `Weights` and `collectorapi.ScoreRuleBelowThreshold` (`score_filter.go`); collector routes for apply, threshold and turn off with their `catalogError` cases; tests in `score_apply_test.go` (evidence with uploader and category weights) and `TestScoreModelValidation`.
