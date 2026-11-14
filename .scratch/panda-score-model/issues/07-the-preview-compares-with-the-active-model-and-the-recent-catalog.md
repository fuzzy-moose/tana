# 07: The preview compares with the active model and the recent catalog

What to build: a preview from the collector also describes the active model and what both models do to the recent catalog. It reports the active model's time, favorites setting and threshold, the number of verdicts newer than it, each weight's value in the active model, and how the newest catalog galleries that pass the default filter fall under the hiding rules: by verdict, by override, and by score under the candidate and under the active model. With this the page can show the share of recent galleries any threshold hides.

This is the server side of spec behaviours 14 (newer verdicts), 15 (the start setting), 16 (active weight), 17 and 20 (recent share), 28 and 52. Its layers are specified in spec **Design**:

- `internal/collectorapi`: `ScorePreview.Active` (`calculated_at`, `favorites`, `threshold_tenths`), `ScoreEvidence.NewVerdicts` (`new_verdicts`, null without an active model), `ScoreWeight.ActiveWeight` (`active_weight`, null without one), `ScorePreview.Recent` (`total`, `interesting`, `not_for_me`, `never_hidden`, `always_hidden`, `candidate`, `active`) and the histogram type (`first_bucket`, `counts`).
- `internal/collector/catalog`: **Design → Preview against the active model and the recent catalog**.
  - `options.Favorites == nil` now means the active model's setting, and `false` without one.
  - The recent sample is the newest `RecentSampleSize` galleries in catalog order that pass the request's default filter and are not expunged. Its predicate comes from ticket 01's builder, called with only the default filter (no search). Each gallery is classed by the rule order of spec behaviour 31, with the current overrides and the SQL score expression of ticket 06, once under the candidate's weights and once under the active model's.
  - Both histograms cover the galleries that rules 1 to 3 don't decide. Bucket `i` counts the galleries with `floor(score * 10) == first_bucket + i`. A histogram is null without its model, and has an empty `counts` when no gallery is left to score.

Test seams: package tests on `pgtest` in `internal/collector/catalog`. The sample size itself is a manual check; don't seed 2000 galleries.

Blocked by: 06

Status: resolved

- [x] Without an active model, `active` is null, `new_verdicts` is null, every `active_weight` is null and `recent.active` is null.
- [x] After an apply, `active` carries the applied model's `calculated_at`, `favorites` and `threshold_tenths`. A weight that the active model also has carries that value as `active_weight`, and a weight the active model lacks carries null.
- [x] A verdict given after the apply, and a verdict changed to the other one after it, each count in `new_verdicts`. Verdicts from before the model's calculation don't.
- [x] With `Favorites` nil and an active model that counts favorites, the preview's `favorites` is true and `counted_favorites` counts them. With `Favorites` nil and no active model, `favorites` is false.
- [x] With a default category in the request, `recent.total` counts only galleries of that category, and with a default query only galleries it matches. An expunged gallery is never counted.
- [x] In `recent`, a gallery with a "Not for me" verdict counts in `not_for_me`, one with an "Interesting" verdict in `interesting`, one that a "never hide" override matches in `never_hidden`, one that only an "always hide" override matches in `always_hidden`, and each remaining gallery in exactly one bucket of `candidate`. The five counts and the histogram's counts add up to `total`.
- [x] A gallery without a verdict or override whose score under the candidate is `s` is counted in the `candidate` bucket `floor(s * 10)`, and under the active model's weights in the matching bucket of `active`.
- [x] Without a candidate (`no_model` set), `recent` still carries the totals and the verdict and override counts, with a null `candidate` histogram.
- [x] For a threshold `t` and an active model, `recent.not_for_me + recent.always_hidden` plus the `active` counts below `t` equals the number of the sample's galleries that `List` with the same default filter leaves out after `SetScoreThreshold(t)`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`collectorapi` gains `ScorePreview.Active`/`Recent`, `ScoreEvidence.NewVerdicts`, `ScoreWeight.ActiveWeight` and `ScoreHistogram`. `PreviewScoreModel` now locks `panda_score_models` first and reads the active model (with its favorites and time) through `readScoreFilter`, defaults `Favorites` to it, counts newer verdicts, fills active weights, and classes the newest `RecentSampleSize` galleries of the default filter in `score_recent.go`; tests in `score_recent_test.go`.
