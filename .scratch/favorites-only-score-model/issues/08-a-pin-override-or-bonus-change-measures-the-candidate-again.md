# 08: A pin, override or bonus change measures the candidate again

What to build: on the score page, an override change, a pin change and an author bonus change no longer start a new calculation. They change no calculated weight, so the request that follows keeps the candidate on screen, with its ID, its calculation time, its weights and their counts, and only measures it again: the two shares, the suggested threshold, the pins' model weights and the favorite authors follow the change. The catalog is not counted again, so these edits answer fast and hold the model lock only briefly. A visit, Retry, Apply, a saved threshold and Turn off calculate as before. When the candidate on screen is no longer stored, the request calculates a new one.

This is spec behaviour 16, the request half of 17, and 32. Fixed in spec **Design → Calculation and measurement → Measuring again**, **`internal/collectorapi`**, **Routes** and **Web (Score request)**. The body changes on all three sides in this one ticket, because `server.DecodeJSON` rejects unknown fields:

- `internal/collectorapi/catalog_score.go`: `ScorePreviewOptions.CandidateID int64` with the JSON name `candidate_id,omitempty`.
- `internal/collector/catalog`: `PreviewScoreModel` with a `CandidateID` that names the stored candidate keeps it and answers its ID, calculation time, weights and counts from the stored rows; it calculates no weight, counts nothing in the catalog and stores no model. Everything else is read at the time of the request, as the spec lists. It takes the same lock and one transaction as a calculation. A `CandidateID` of 0, or one that names no stored candidate (unknown, or the active model's), calculates as before; that is no error.
- `internal/collector/httpapi`: no change beyond the option.
- `internal/local/httpapi/catalog_score.go`: `HandleCollectorScorePreview` decodes `{}` or `{"candidate_id": 7}` and sends the ID with the default filter.
- `web/src/panda/score.ts`, `PandaScorePage.tsx`: `previewPandaScore(candidateID: number | null, signal)`. The page sends the ID of the candidate it shows with the request that follows an override change, a pin change or an author bonus change, and `{}` when it shows no candidate. The first request of a visit, Retry, and the request that follows Apply, a saved threshold, Turn off or a failed Apply send `{}`.

Test seams: `PreviewScoreModel` in `internal/collector/catalog` with favorites as in ticket 04; `TestScorePreviewValidation`; `TestLocalScoreModel`, which captures the body sent to the collector; `web/src/panda/PandaScorePage.test.tsx` with a mocked `fetch`. The web tests of ticket 03 that expect `{}` after an override, pin or bonus change now expect the ID.

Hints (optional): a catalog gallery that is added between the calculation and the measurement and carries a weighted value would change that value's `Catalog` count in a new calculation; in a measurement the count stays. A value without a weight in the candidate has none in a held-out score, so the stored weight rows and the two totals are enough.

Blocked by: 05, 06

Status: resolved

- [x] A preview with the `CandidateID` of the stored candidate answers that candidate: the same `ID`, `CalculatedAt` and `WeightCount`, and weights with the same `Weight`, `Favorites`, `Catalog` and order. `panda_score_models` holds the same candidate row afterwards and no new one, and the active model with its weights is unchanged.
- [x] It counts nothing again: after a catalog gallery with a weighted value and a favorite with that value were added since the calculation, the answer's weights and counts are those of the calculation, while a request without `CandidateID` gives a new candidate with the changed counts.
- [x] It measures with the current pins, overrides and bonus: with unchanged favorites and catalog, after a pin, an override and an author bonus change, its `HeldOut`, `SuggestedThresholdTenths`, `Recent`, `Pins` with their `ModelWeight`, `Overrides`, `AuthorBonus` and `FavoriteAuthors` equal those of a new calculation.
- [x] The evidence numbers are current: a favorite in scope that was added since the calculation counts in `Evidence.Favorites` and in `HeldOut.Total`.
- [x] A `CandidateID` of 0, an unknown one, and the ID of a candidate that was applied since each calculate a new candidate with a new ID, as a request without the field does.
- [x] The collector answers `POST /api/catalog/score/preview` with 200 for a body with `default_query`, `default_categories` and `candidate_id`.
- [x] Tana answers `POST /api/collector/catalog/score/preview` with body `{"candidate_id":7}` by sending the collector `candidate_id` 7 beside `default_query` and `default_categories` from the saved default filter. With body `{}` it sends no `candidate_id`.
- [x] On the score page, the request that follows an override change (row, list, form), a pin change (row, list, form, author list) and an author bonus change has the body `{"candidate_id":<id of the candidate on screen>}`. When the page shows no candidate, the body is `{}`.
- [x] The first request of a visit, Retry, and the request that follows Apply, a saved threshold, Turn off and an Apply that failed with a replaced candidate have the body `{}`.
- [x] When the answer to such a request carries another candidate ID, the page shows that candidate and Apply sends its ID.
- [x] The `go` and `frontend` checks of the green check pass, and `gofmt -l .` prints nothing.

## Resolution

`PreviewScoreModel` takes `ScorePreviewOptions.CandidateID`: when it names the stored candidate (not the active model), the preview reads that candidate's weights, counts and totals from `panda_score_models` and `panda_score_weights` (`storedCalculation`), calculates and stores nothing, and measures it with the current favorites, pins, overrides, bonus and recent sample. A measurement keeps the candidate even when no favorite is in scope any more. Otherwise it calculates as before. Tana's preview route forwards `candidate_id` with the default filter. `previewPandaScore(candidateID, signal)` sends the ID of the candidate on screen after an override, pin or bonus change, and `{}` otherwise. Tests: `score_measure_test.go`, `TestScorePreviewValidation`, `TestLocalScoreModel`, `PandaScorePage.test.tsx`.
