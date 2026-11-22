# 04: The preview counts every favorite and gallery by its score

What to build: the numbers of the evaluation page have no "always hidden" or "never hidden" part: every favorite in scope gets a held-out favorite score, every gallery of the recent sample counts by its score, and without an active model 0 % is hidden now. Spec behaviours 6, 7 and 12; shape in spec **Design → Preview**.

The collector, `internal/collectorapi`, the share functions of `web/src/panda/score.ts` and the web tests' preview fixtures change together (spec **Fit** 3), and the code that matches an override against a gallery goes. The preview lists `overrides` until ticket 06.

Blocked by: 02

Status: resolved

- [x] In a preview, `held_out.total` is the number of favorites in scope and the sum of `held_out.scores.counts`.
- [x] The suggested threshold is the highest that hides at most 5 % of all held-out favorites, by score.
- [x] `recent.total` equals the sum of the counts of each histogram the preview carries.
- [x] With an active model, `HeldOutFavorites` of a catalog page is the number of favorites in the model's scope, and the score grades compare with all their scores.
- [x] `hiddenShare` and `recentHiddenShare` return the buckets below the threshold divided by `total`, and 0 for a total of 0. `currentHiddenShare` returns the active model's share at its threshold, and 0 without one.
- [x] `grep -rnE "(always|never)_hidden|(Always|Never)Hidden" internal web/src` prints nothing.
- [x] `go vet ./...`, `go test ./...` and, in `web/`, `npm test`, `npm run lint` and `npm run build` pass; `gofmt -l .` prints nothing.

## Resolution

`heldOutScores` scores every favorite in scope and `recent` counts every sampled gallery by score; `ScoreHeldOut` and `ScoreRecent` (collectorapi) and `HeldOut`/`RecentCatalog` (web) lost `always_hidden`/`never_hidden`, `suggestedThreshold` counts from the histogram only, and `scoreFilter.matches`/`decidingOverride` are gone. `hiddenShare`/`recentHiddenShare` divide the buckets below the threshold by `total`, and `currentHiddenShare` is 0 without an active model. Override tests of the held-out, recent, measurement and grade numbers now use -100/+1000 pins, and a +1000 pin is shown to get the top score grade.
