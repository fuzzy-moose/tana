# 02: The listing hides by the score only

What to build: the collector's catalog page hides a gallery only when its Panda gallery score is below the score threshold, and the score filter is in force only while a Panda score model is active. Spec behaviours 8, 9 and 13; shape in spec **Design → Catalog page**.

Override storage, API and preview numbers stay for later tickets. Tests of behaviour that stays get an active model instead of an override (spec **Fit** 2). Three of them cannot reach the catalog test helpers:

- `TestCatalogIgnoresVerdictParameter` (collector `httpapi`) and `TestLocalCatalogScoreFilter` (local `httpapi`) hide a gallery with a model row seeded by SQL, as `storeScoreCandidate` writes it, and a pin.
- `TestScoreModelsFromFavoritesMigrationDeletesEveryModel` (`storage`) asserts no score filter: its migration leaves no model. Its count of `panda_score_overrides` stays until ticket 07.

Blocked by: None

Status: resolved

- [x] Without an active model, also with pins and an author bonus stored, a catalog page has no `ScoreFilter`, no `Score` and no tag weight, lists every gallery, and its `hidden` state is empty.
- [x] With an active model the filtered page lists exactly the galleries scoring at or above the threshold, and the `hidden` state the others. Opposite pins on one gallery add up in its score.
- [x] An item below the threshold reports the rule `below_threshold`, every other item none, and no item an override, also through Tana's `GET /api/collector/catalog`.
- [x] Each part of behaviour 13 passes a test with a model that hides galleries.
- [x] `go vet ./...` and `go test ./...` pass, and `gofmt -l .` prints nothing.

## Resolution

The score filter is in force only with an active model (`scoreFilter.inForce`), `hidden` is the threshold comparison alone, and `markScores` gives every item its score and only the rule `below_threshold`; `collectorapi.CatalogItemScore.Override` is gone. Listing tests hide with a model (`hideBy` in `score_filter_test.go`), the override-order tests became `TestScoreFilterAddsOppositePins`, `TestRemoveScoreModelKeepsOverrides` became `TestRemoveScoreModelTurnsTheFilterOff`, and the three tests outside the catalog package apply a SQL-seeded model with a pin.
