# 05: The score preview counts pinned weights

What to build: the preview of the evaluation page lists the pins, and every number it gives about the candidate or the listing counts them: the held-out scores of the evidence galleries, the suggested threshold, and the scores of the recent catalog galleries under the candidate and under the active model. The page's shares and error rates then describe what the listing does with the pins. A pin of any permitted size leaves the preview working, so the user can always open the evaluation page and remove it. The page itself shows the pins in ticket 11.

This is the pin side of spec behaviour 35 and the preview's pins of behaviour 54. Its layers are specified in spec **Design**:

- `internal/collectorapi/catalog_score.go`: `ScorePreview.Pins []ScorePin` (`json:"pins"`), every pin by ID, never nil.
- `internal/collector/catalog`: **Design → Pins in the preview (ticket 05)** and the Go side of **Design → The weight rule** (without the favorite author part, which is ticket 08). `PreviewScoreModel` fills `Pins`. `heldOutTenths` scores each evidence gallery with the effective weights of its fold: a pinned tag, uploader or category counts with its pin, whatever the fold calculated for it, and also when the fold has no weight for it. One private Go function holds the rule. The window of ticket 02 stays: `heldOutTenths` limits the tenths after it has summed the effective weights. `scoreFilter.recent` already scores through the builder of ticket 04 and inside the window of ticket 02, so it needs a test and no code. `suggestedThreshold` and `scoreFilter.heldOut` read the held-out scores and do not change. `Candidate.Weights` keeps the calculated weights and their order.
- No route is added, and the handlers need no code: the new field passes through `HandlePreviewScoreModel` and `HandleCollectorScorePreview` on the shared struct.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `seedVerdicts`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate`, `previewScore`, `weightOf` and `histogramCounts`. New assertions in `TestLocalScoreModel` (`internal/local/httpapi/catalog_test.go`) or a test beside it. Existing assertions stay unchanged.

Hints (optional): in `seedScoreEvidence`, galleries 1 to 5 are "Not for me" with `female:glasses`, and galleries 6 to 10 are "Interesting" with `male:hat`. Every other value of these galleries is carried by at most three of them, so the fold that scores one of its galleries has no weight for it (`MinSupport`). A score of 100 is bucket 1000. With `male:hat` pinned to 100, every "Interesting" evidence gallery has a held-out score of at least 100.

Blocked by: 02, 04

Status: resolved

- [x] `PreviewScoreModel` returns every stored pin in `Pins`, in the order of their IDs, each with its ID, kind, label and weight. Without pins, `Pins` is empty and the preview's JSON holds `"pins":[]`.
- [x] The held-out score of an evidence gallery counts a pin in place of the weight that its fold calculated: a pinned tag counts with its pin in the score of each evidence gallery that carries it, a pin of 0 removes the tag's weight from those scores, and a pin on a value that the fold has no weight for counts as well.
- [x] The suggested threshold follows the pins: with a large positive pin on a tag that every "Interesting" evidence gallery carries, `Candidate.SuggestedThresholdTenths` rises with the held-out scores.
- [x] The recent scores count a pin under the candidate and under the active model: a recent gallery that carries a pinned tag is counted in the bucket of its pinned score in `Recent.Candidate` and, once a model is active, in `Recent.Active`. After the pin is removed, the next preview counts it in the bucket of its calculated score.
- [x] `Candidate.Weights` holds the calculated weights while pins are stored: a pinned value keeps its calculated `Weight` there.
- [x] Pins of any size leave the preview working: with one tag pinned to -1e6 and another to 1e12, each carried by evidence galleries and by recent galleries, `PreviewScoreModel` succeeds, counts those galleries in bucket -10000 or 10000, has no histogram with more than 20001 counts, encodes to less than 1 MiB of JSON, and `ApplyScoreModel` accepts its `SuggestedThresholdTenths`.
- [x] Through the real collector handler, the answer of local `POST /api/collector/catalog/score/preview` holds the stored pin under `pins` with `id`, `kind`, `label` and `weight`.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`ScorePreview.Pins` lists the stored pins; `PreviewScoreModel` fills it and `heldOutTenths` scores each evidence gallery with the pins over its fold's weights through `effectiveWeight` (`internal/collector/catalog/score_held_out.go`), tested in `score_pins_preview_test.go` and `TestLocalScoreModel`.
