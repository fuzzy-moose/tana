# 02: Prefactor: Score histograms count inside a fixed score window

What to build: nothing the user sees changes. A preview gives the evaluation page five histograms of scores: three of the held-out evidence and two of the recent catalog galleries. Each has one count for every tenth between its lowest and its highest score. Today a score is a sum of calculated weights and stays small. Pins and the author bonus (tickets 04 to 08) allow a score of any size. One such score would make a histogram millions of counts wide, fail the conversion of its tenths to an integer, and fail the preview, and then the evaluation page, the only place that lists the pins, could not load (spec **Fit**, item 2). From this ticket on, every histogram counts inside a fixed window of scores, and a score beyond the window counts at its edge.

Its parts are specified in spec **Design → Limits** (the window) and **Design → Collector: `internal/collector/catalog`**, **Ticket 02**:

- `const scoreWindowTenths = 10000` in `internal/collector/catalog`, with a comment on its purpose: the histograms of a preview count scores from -1000.0 to 1000.0.
- The tenths that a histogram counts are `floor(score * 10)` limited to `[-scoreWindowTenths, scoreWindowTenths]`. `scoreFilter.recent` limits them in SQL for the candidate and for the active model, before the cast to `integer`. `heldOutTenths` limits them in Go, before the conversion to `int`.
- The lowest bucket of a histogram then also counts every lower score, and the highest bucket every higher one. `scoreHistogram`, `suggestedThreshold` and `scoreFilter.heldOut` need no change: they get tenths inside the window.
- The listing has no window. `belowThreshold`, `hidden` and `markScores` keep the unlimited `scoreTenths` and `scoreSum`.
- No exported name changes, and `collectorapi.ScoreHistogram` keeps its shape. Its comment gains one sentence: the first and the last bucket of a histogram that reaches the window's edge also count the scores beyond it.

Test seam: package tests in `internal/collector/catalog` with `openCatalog`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate`, `previewScore` and `histogramCounts`. No existing test changes.

Hints (optional): a recent gallery gets a score beyond the window when a test changes a weight of the applied model in `panda_score_weights` directly. An evidence gallery gets a held-out score beyond the window when it carries a few thousand tags that each have a weight in its fold. `heldOutTenths` takes plain values, so this needs no database.

Blocked by: None

Status: resolved

- [x] A recent catalog gallery whose score under the active model is above 1000 is counted in bucket 10000 of `Recent.Active`, and one whose score is below -1000 in bucket -10000. The preview succeeds also for a score of 1e12, and none of its histograms has more than 20001 counts.
- [x] `heldOutTenths` gives an evidence gallery whose held-out score is beyond the window the tenths of the window's edge, and every other gallery `floor(score * 10)` as before.
- [x] Inside the window nothing changes: every existing test of `internal/collector/catalog`, `internal/collector/httpapi` and `internal/local/httpapi` passes, and the commits of this ticket change no existing test.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Added `scoreWindowTenths` in `internal/collector/catalog`: `scoreFilter.recent` clamps the candidate and active tenths in SQL (`windowTenths`) and `heldOutTenths` clamps in Go, so every preview histogram spans at most 20001 buckets; new tests in `score_recent_test.go` and `score_held_out_test.go`, and the `collectorapi.ScoreHistogram` comment names the edge buckets.
