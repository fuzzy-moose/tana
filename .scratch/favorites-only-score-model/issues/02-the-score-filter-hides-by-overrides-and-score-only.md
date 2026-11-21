# 02: The score filter hides by overrides and score only

What to build: a verdict no longer decides whether the score filter hides a gallery. The listing hides a gallery by a "never hide" override, an "always hide" override and the score below the threshold, in that order, and by nothing else. A gallery marked "Not for me" is listed again unless one of these rules hides it. Verdicts stay stored until ticket 09 and stay evidence for the model until ticket 04.

This is spec behaviour 6 (collector side) and the listing part of behaviour 14. Layers:

- `internal/collector/catalog/score_filter.go`: the hiding predicate keeps the three rules of behaviour 6 and no longer reads verdicts. Its comments and the comment of `collectorapi.CatalogItemScore` no longer speak of a verdict. No route and no payload changes.
- Test work that belongs to this ticket (spec **Fit** 1 and **Design → Test seams → Stored model and stored candidate**): the tests of what the listing and the model lifecycle do stop giving verdicts. Today they build their models with `seedVerdicts`/`seedScoreEvidence` and `applyCandidate`, and `internal/local/httpapi/catalog_test.go` with `service.SetVerdict` and a preview. After this ticket they store their models and candidates directly in `panda_score_models` and `panda_score_weights`, so they no longer depend on how a model is calculated. This is what lets ticket 04 replace the calculation without touching them.

Which tests move to stored rows:

- Listing: the listing tests in `score_apply_test.go` (hiding below the threshold, threshold change, turn off, provisional then collected tags, hidden galleries stay unseen), all of `score_pins_listing_test.go`, the listing tests of `score_author_bonus_test.go`, the model tests of `summary_tags_test.go`, `score_tag_weights_test.go`, and `TestLocalCatalogSummaryTagsRankWeightedTags`, `TestLocalCatalogMarksPinnedWeights` and `TestLocalAuthorBonus`.
- Lifecycle: the apply, missing-candidate, waiting, concurrent-apply and second-candidate tests of `score_apply_test.go`. `TestConcurrentAppliesLeaveOneActiveModel` and `TestScoreModelValidation` already insert candidates directly. Where a test needs "a later preview replaced the candidate", a preview on a database without evidence does that: it takes the lock, deletes every candidate and stores none, before and after ticket 04.

Which tests keep verdicts for now: tests of the preview itself (`score_model_test.go`, `score_recent_test.go`, `score_held_out_test.go`, `score_pins_preview_test.go`, `score_author_bonus_preview_test.go`) and `TestLocalScoreModel`, until ticket 04; tests of verdict storage and verdict views (`verdicts_test.go`, the verdict-view listings inside other tests), until ticket 09. `TestVerdictViewIgnoresTheActiveModel` goes now: it takes its model from a verdict preview and tests a view that ticket 09 removes.

Hints (optional): the weights that `seedScoreEvidence` documents are exactly `ln 0.5` for `female:glasses`, `ln 0.625` for each artist tag and the uploader "Bad One", `ln 2` for `male:hat` and `ln 1.6` for the category `manga`. Stored with the galleries 1 to 10, they leave most expectations as they are. Expected listings change only where a verdict decided: galleries 1 to 10 are now hidden or shown by their score. The model row still needs `favorites`, and each weight row `interesting`, `not_for_me` and `neutral`, until tickets 03 and 04 drop these columns.

Blocked by: None

Status: resolved

- [x] The rule order holds: a "never hide" match shows a gallery that an "always hide" override or its score would hide; an "always hide" match hides a gallery whose score is above the threshold; a gallery without an override match is hidden exactly when its score is below the threshold, in "Filter on", and listed in "Hidden only" exactly then. `CatalogItem.Score.Rule` and `Override` name the deciding rule as before.
- [x] The listing tests named above pass with a model that the test stores and applies itself: none of them calls `seedVerdicts`, `seedScoreEvidence` or `PreviewScoreModel` to get its model, and the pin, author bonus, summary-tag and tag-weight expectations they held before still hold.
- [x] The lifecycle tests named above pass with candidates that the test stores itself: an apply makes the candidate the active model with its threshold and deletes the model that was active with its weights; an unknown ID, the active model's ID and the ID of a replaced candidate give `ErrScoreCandidateNotFound` and leave the active model and the listing unchanged; an apply waits for a transaction that holds the preview's lock; two applies at once leave one active model.
- [x] Through the real collector handler, the three local tests named above get their active model the same way and keep their assertions on `summary_tags`, pins and the author bonus.
- [x] Removal: `internal/collector/catalog/score_filter.go` contains no "verdict" in any letter case, and `TestVerdictViewIgnoresTheActiveModel` no longer exists. After this ticket a test in the files named above gives a verdict only to list a verdict view; ticket 09 removes those.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

`scoreFilter.hidden` in `internal/collector/catalog/score_filter.go` hides by a "never hide" override, an "always hide" override and the score below the threshold only, and its comments and the `collectorapi.CatalogItemScore` comment no longer speak of a verdict. The listing and lifecycle tests in `internal/collector/catalog` store their models and candidates with `storeScoreCandidate`/`applyScoreModel` (new `score_stored_test.go`, over `seedScoreGalleries` and `storedScoreWeights`), and the three local tests use `applyTagModel`. `TestScoreFilterRuleOrder` now checks the rule order against a stored model, the preview helpers (`seedVerdicts`, `seedScoreEvidence`, `applyCandidate`, `weightOf`) moved to `score_model_test.go` for the preview tests that keep them until ticket 04, and `TestVerdictViewIgnoresTheActiveModel` is gone. `TestPreviewRecentClassesGalleriesByTheHidingRules` compares the recent sample with the hidden listing items that have no verdict, since the sample still classes those by verdict until ticket 04.
