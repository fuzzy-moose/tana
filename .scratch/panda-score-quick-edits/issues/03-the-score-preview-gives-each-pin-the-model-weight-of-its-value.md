# 03: The score preview gives each pin the model weight of its value

What to build: each pin of a score preview says which weight the new model calculated for the pinned value. Ticket 05 shows this number in the "Pinned weights" list, so that the user sees what a pin replaces. The page cannot find it on its own: a preview carries only the 500 strongest weights, and a pin usually sits on a value with a weak weight or none.

This is the collector side of spec behaviour 12, specified in spec **Design → Collector → Model weight of a pin**, **Design → `internal/collectorapi`** and **Fit** 2.

- `internal/collectorapi/catalog_score.go`: `ScorePin` gains `ModelWeight *float64` with the JSON key `model_weight,omitempty`. Its comment says that the field is set in a preview only.
- `internal/collector/catalog/score_model.go`: `PreviewScoreModel` sets `ModelWeight` on each pin of `ScorePreview.Pins`: the weight that the candidate of this preview calculated for the pin's value, nil when the candidate has none, and nil when the preview has no candidate. Read it from all weights of the candidate (the map that `scoreWeights` returns), not from `candidate.Weights`, which holds only the 500 strongest. A pin is keyed as `pinnedWeights` keys it: a tag by its `catalog_tags` ID, an uploader or a category by its label.
- Nothing else sets the field: the answer of `SetScorePin` and the pin of a tag weight lookup leave the key out. The local service decodes and encodes `collectorapi.ScorePreview`, so `internal/local/httpapi` needs no change.

Test seam: the tests of `internal/collector/catalog` on PostgreSQL through `pgtest` (`score_pins_preview_test.go`), with `seedScoreEvidence`, `seedVerdicts`, `setPin`, `previewScore` and `weightOf`. `TestPreviewListsThePins` compares the pins of a preview with the answers of `setPin`; it changes because its pins now carry model weights. The number of weights that `Candidate.Weights` lists is `scorePreviewWeights` (500).

Blocked by: None

Status: resolved

- [x] In a preview with a candidate, a pin on a value that the candidate weighs has `ModelWeight` equal to that value's weight in `Candidate.Weights`, for a tag pin, an uploader pin and a category pin.
- [x] A pin on a value without a weight in the candidate (a catalog tag that no evidence gallery carries) has a nil `ModelWeight`, and so has every pin of a preview without a candidate.
- [x] A pin on a value whose weight the candidate has but `Candidate.Weights` does not list (a model with more than 500 weights) has `ModelWeight`.
- [x] The JSON of a preview names the field `model_weight` on the pin and leaves it out for a nil value. The pin that `SetScorePin` returns encodes without the key.
- [x] `go vet ./...` and `go test ./...` pass, and `gofmt -l .` prints nothing.

## Resolution

`collectorapi.ScorePin` gained `ModelWeight` (`model_weight,omitempty`); `PreviewScoreModel` sets it on each pin from all weights of the candidate, keyed by the new `scorePin.value()` that `pinnedWeights` also uses. Tests in `score_pins_preview_test.go` cover tag, uploader and category pins, a value without a weight, a preview without a candidate, a value beyond the 500 listed weights, and the JSON key.
