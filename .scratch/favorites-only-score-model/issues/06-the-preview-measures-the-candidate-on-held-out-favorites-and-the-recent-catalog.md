# 06: The preview measures the candidate on held-out favorites and the recent catalog

What to build: the preview says what a threshold would do. For each favorite in scope it gives the score the favorite would have if the weights came from the other favorites only, so the score page shows the share of held-out favorites that a threshold hides in place of "0 %". It suggests the highest threshold that hides at most 5% of them, so the page's "Suggested" value is real. The recent catalog sample, which ticket 04 left real but without tests, is pinned again.

The page already shows these numbers (ticket 05), so this ticket changes the collector only.

This is spec behaviours 25 to 28 (collector side) and the measurement part of 14. Fixed in spec **Design → Calculation and measurement** (Held-out favorites, Suggested threshold, Recent sample):

- `internal/collector/catalog`: the held-out score of each favorite in scope, `Candidate.HeldOut`, and `Candidate.SuggestedThresholdTenths` with `SuggestedFavoriteLoss = 0.05`. The held-out weight of a value comes from its two counts and the two totals alone, which the candidate stores, so ticket 08 can measure a stored candidate again.
- The author bonus is held out too: the tag of a favorite author weighs at least the bonus in a held-out score only when its stored `favorite_authors.favorites` is at least 3.
- No route and no payload shape changes.

Test seams: `PreviewScoreModel` in `internal/collector/catalog` with favorites as in ticket 04, pins, overrides, the author bonus and rows in `favorite_authors`; a unit test of the suggested-threshold rule; a stored active model (ticket 02) or a real apply for the recent sample under the active model; one assertion through the real collector handler in `internal/local/httpapi/catalog_test.go`.

Hints (optional): a value that exactly 3 favorites carry has a candidate weight, and no weight in the held-out score of those three, because 2 favorites are under `MinSupport`. Scores are counted as `floor(score * 10)`. A pin of 100 on a tag that every favorite carries puts every held-out score at bucket 1000 or above.

Blocked by: 04

Status: resolved

- [x] `Candidate.HeldOut.Total` is the number of favorites in scope, and `Total` equals `AlwaysHidden + NeverHidden` plus the sum of `Scores.Counts`.
- [x] A held-out score leaves the favorite's own contribution out: a value that exactly 3 favorites carry adds nothing to the held-out scores of those three, and a value that more favorites carry counts with the weight calculated from `f - 1` and `F - 1` and the unchanged catalog counts.
- [x] Held-out scores count pins as the listing does: a pinned value counts with its pin, a pin of 0 removes the value, and a pin on a value without a calculated weight counts.
- [x] With the author bonus on, the tag of a favorite author whose stored number of favorites is 3 or more counts at least the bonus in the held-out score of a favorite that carries it. The tag of a favorite author with exactly 2 favorites gets no bonus there: it counts with its held-out weight alone, which is none when only those two favorites carry it. A pin on the tag wins in both cases.
- [x] A held-out favorite that matches a "never hide" override is counted in `NeverHidden`, also when an "always hide" override matches too; one that matches only an "always hide" override is counted in `AlwaysHidden`; neither is in `Scores`.
- [x] `SuggestedThresholdTenths` is the highest threshold that hides at most 5% of the held-out favorites, counting `AlwaysHidden`; it is the lowest scored bucket when no threshold does, and 0 without scored favorites. It rises with a large positive pin on a tag that every favorite carries, and `ApplyScoreModel` accepts it.
- [x] A held-out score beyond the window is counted at its edge: with pins of -1e6 and 1e12 on tags of favorites, the histogram has at most 20,001 counts and the preview succeeds.
- [x] `Recent` classes the newest galleries that the listing shows with the default filter (expunged galleries left out, a default category applied) by the first rule that decides: a "never hide" override, an "always hide" override, else the score under the candidate in `Recent.Candidate` and under the active model in `Recent.Active`.
- [x] The recent scores count pins and the author bonus under the candidate and under the active model, and a score beyond the window at its edge.
- [x] Through the real collector handler, Tana's preview answer holds `candidate.held_out.total` and `candidate.suggested_threshold_tenths` as the collector calculated them.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

The preview scores each favorite in scope with weights held out from it (`score_held_out.go`: `heldOutWeight` from the stored counts, pins, the author bonus only for authors with at least 3 stored favorites, overrides first) into `Candidate.HeldOut`, and sets `SuggestedThresholdTenths` with `SuggestedFavoriteLoss`; tests in `score_held_out_test.go`, a recent-sample test under the candidate and the active model, and an assertion in `TestLocalScoreModel`.
