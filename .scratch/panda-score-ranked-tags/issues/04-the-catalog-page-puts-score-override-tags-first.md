# 04: The catalog page puts score override tags first

What to build: a catalog page from the collector offers each card the gallery's tags that have a score override before all other tags, "never hide" before "always hide", each with its rule. This holds with and without an active Panda score model. In "Filter off" and "Hidden only", the first tag of a card is then the tag that hides or keeps the gallery. The mark on the card comes in ticket 05.

This is spec behaviours 12, 13 and 15, and the collector side of behaviours 7, 16, 18 and 20 for override tags. Its layers are specified in spec **Design**:

- `internal/collectorapi/catalog.go`: `CatalogTag.Override` (`json:"override,omitempty"`), which holds `ScoreRuleAlwaysHide` or `ScoreRuleNeverHide`.
- `internal/collector/catalog`: the first group of **Design → `internal/collector/catalog`**. The summary lookup reads the tag overrides from `scoreFilter.overrides`, which `List` has read already (entries of kind tag, with `TagID` and `Rule`), and runs no further statement for them. An override tag keeps its `Weight` when the active model has one for it. Uploader and category overrides play no part. `markScores` and `decidingOverride` do not change.
- No route is added, and the handlers need no code: the new field passes through on the shared struct.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `setVerdict`, `setOverride`, `scorePage`, `itemByID`, `unseenPage`, `seedScoreEvidence` and `applyCandidate`. New assertions in `TestLocalCatalogScoreFilter` (`internal/local/httpapi/catalog_test.go`), whose fixture already has an "always hide" override for `female:big hat`. Existing assertions stay unchanged.

In the criteria with a model, the weights are those of `seedScoreEvidence`: `female:glasses` about -0.69, `artist:a` about -0.47, `male:hat` about 0.69.

Blocked by: 02

Status: resolved

- [x] Without a model, with an "always hide" override for `male:hat`, a gallery with the tags `artist:u1`, `female:big hat` and `male:hat` has, in the states `"all"` and `"hidden"`, the `SummaryTags` `male:hat`, `artist:u1`, `female:big hat`. The first has `Override` `"always_hide"`. The others have an empty `Override`, and all three have a nil `Weight`.
- [x] Without a model, with an "always hide" override for `artist:u1` and a "never hide" override for `male:hat`, a gallery with the tags `artist:u1`, `female:big hat` and `male:hat` is listed in the state `""` with the `SummaryTags` `male:hat` (`"never_hide"`), `artist:u1` (`"always_hide"`), `female:big hat`. With "always hide" overrides for both `artist:u1` and `male:hat`, the gallery has them in today's order in the state `"all"`: `artist:u1`, `male:hat`, `female:big hat`.
- [x] Without a model, a gallery with `artist:u1` to `artist:u6` and `male:hat`, with an override for `male:hat`, has the `SummaryTags` `male:hat`, then `artist:u1` to `artist:u5`.
- [x] With the model of `seedScoreEvidence` applied and a "never hide" override for `other:plain`, a gallery with the tags `artist:a`, `female:glasses` and `other:plain` has the `SummaryTags` `other:plain` (`"never_hide"`, nil `Weight`), `female:glasses`, `artist:a`: a tag with an override comes before the strongest weighted tag.
- [x] With that model and an "always hide" override for `artist:a` as the only override, a gallery with the tags `artist:a` and `female:glasses` has, in the state `"all"`, `artist:a` first, with `Override` `"always_hide"` and a `Weight` equal to the candidate's weight for it, then `female:glasses` with its weight and an empty `Override`.
- [x] With only an override for an uploader and an override for a category, a gallery that both match has its first six tags in today's order, each with an empty `Override`, although its `Score` names an override.
- [x] A gallery with a "never hide" tag has that tag first, with its `Override`, in the unseen view. A gallery with an override tag and a "Not for me" verdict has that tag first in the "Not for me" view.
- [x] After `SetScoreOverride` gives the tag the other rule, the next `List` shows the new rule in `Override`. After `RemoveScoreOverride`, the next `List` gives the tag an empty `Override` and the place its weight or today's order gives it.
- [x] Through the real collector handler, the local `GET /api/collector/catalog?score_filter=hidden` page of `TestLocalCatalogScoreFilter` has `"override": "always_hide"` on the first `summary_tags` entry of its item.
- [x] The page of the local gallery list is as before: the existing tests of `internal/local/gallery` and `internal/local/httpapi` pass unchanged.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`collectorapi.CatalogTag` gained `Override`; `summarizeTags` in `internal/collector/catalog/catalog.go` reads the tag overrides from the score filter and ranks override tags ("never hide" first) before weighted tags. Tests in `summary_tags_test.go` and `TestLocalCatalogScoreFilter`.
