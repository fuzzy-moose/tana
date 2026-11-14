# 08: The preview measures the candidate on held-out evidence

What to build: a preview from the collector says how well the candidate separates the user's evidence, measured only on galleries the tested model did not learn from. For each evidence class it reports how many galleries the default filter passes, how many an override always hides or never hides, and the distribution of held-out scores of the rest. With this the page can show, for any threshold, the share of "Not for me", "Interesting" and neutral galleries it hides, and the two error rates. The preview also suggests a start threshold.

This is the server side of spec behaviours 17 (evidence shares), 18, 28, 51 and 52. Its layers are specified in spec **Design**:

- `internal/collectorapi`: `ScoreCandidate.HeldOut` (`held_out` with `interesting`, `not_for_me`, `neutral`; each `total`, `always_hidden`, `never_hidden`, and `scores` as the histogram type of ticket 07) and `ScoreCandidate.SuggestedThresholdTenths` (`suggested_threshold_tenths`).
- `internal/collector/catalog`: **Design → Held-out effect**.
  - Folds are `gallery_id % 5`. For each fold the weights come from the counts of the other four folds, through the weight functions of ticket 05, `MinSupport` included. Each evidence gallery is scored with the weights of the model that left its fold out.
  - Constraints: the held-out scores come from the same reading of the evidence as the counts and the candidate's weights, and no fold weight is stored after the transaction.
  - The evaluation set is the evidence galleries that pass the request's default filter and are not expunged (ticket 01's builder). Rules 2 and 3 apply with the current overrides. Rule 1 does not: every gallery with a verdict would be decided by it.
  - The suggested threshold is a pure function of the "Interesting" class effect and `SuggestedInterestingLoss`.

Test seams: plain table tests for the suggested threshold. Package tests on `pgtest` in `internal/collector/catalog`. Gallery IDs choose the fold, so a test can place galleries in a fold on purpose.

Blocked by: 07

Status: resolved

- [x] For each class, `total` is the number of that class's evidence galleries that pass the request's default filter and are not expunged, and `always_hidden`, `never_hidden` and the histogram's counts add up to `total`.
- [x] An evidence gallery that only an "always hide" override matches counts in `always_hidden` of its class, one that a "never hide" override matches in `never_hidden`, also when both match. A gallery with a "Not for me" verdict and no override is counted in the `not_for_me` histogram, not as hidden by its verdict.
- [x] A tag that only galleries of one fold carry adds nothing to the held-out scores of those galleries: each lands in the bucket of the score from its other tags, uploader and category under weights calculated without its fold, although the candidate's own weight for that tag is not 0.
- [x] A gallery in a fold whose other four folds hold no "Not for me" evidence lands in bucket 0.
- [x] With evidence in which one tag marks the "Not for me" galleries of every fold and another the "Interesting" galleries of every fold, every scored "Not for me" gallery lies in a lower bucket than every scored "Interesting" gallery.
- [x] With a default category in the request, an evidence gallery of another category is in no class total, and its tags still count in the candidate's weights.
- [x] The suggested threshold function returns the highest threshold within the "Interesting" histogram's range at which the hidden share of "Interesting" (`always_hidden` plus the buckets below it, over `total`) is at most `SuggestedInterestingLoss`, the histogram's first bucket when no threshold in the range meets that, and 0 when the histogram is empty.
- [x] `candidate.suggested_threshold_tenths` in a preview equals that function's result for the preview's own `held_out.interesting`.
- [x] After a preview, `panda_score_weights` holds only the weights of the candidate and of the active model.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`collectorapi.ScoreCandidate` gains `HeldOut` (`ScoreHeldOut` of three `ScoreClassEffect`) and `SuggestedThresholdTenths`; `internal/collector/catalog/score_held_out.go` scores each evidence gallery with fold weights computed in memory from the same evidence reading (`gallery_id % 5`), classes the evidence that passes the default filter by rules 2 and 3, and derives the suggested threshold with `suggestedThreshold` and `SuggestedInterestingLoss`; tests in `score_held_out_test.go`.
