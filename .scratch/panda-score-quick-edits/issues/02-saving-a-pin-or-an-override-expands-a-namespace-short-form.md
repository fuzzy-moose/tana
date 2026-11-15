# 02: Saving a pin or an override expands a namespace short form

What to build: a tag pin or a tag override whose namespace is typed as a short form is saved for the full tag. `f:big breasts` in "Add pin" pins `female:big breasts`, and the same text in "Add override" overrides it. Everything else about saving stays: the tag has to be in the Panda catalog, the namespace and the value match in any case, and `_` stands for a space.

This is spec behaviours 32 and 33, specified in spec **Design → Collector → Short forms on save** and **Fit** 4.

- `internal/collector/catalog`: `SetScorePin` and `SetScoreOverride` expand the namespace of a tag target after it is lowercased. A namespace that is a key of `gallerysearch.NamespaceShortForms` becomes that map's value. The expansion is unconditional.
- `ScoreTagWeights` shares `parseScoreTarget` with the two and must not expand: it keeps matching a sent tag by its namespace as sent. How the two save paths get the expansion without the lookup getting it is yours (a step after `parseScoreTarget`, a parameter, a second helper). Correct the comments of `parseScoreTarget` and `ScoreTagWeights` where they say that the lookup matches like a pin.
- No signature, route, status code or error code changes. `tag_not_found` and `invalid_pin` or `invalid_override` answer as today.

Test seam: the tests of `internal/collector/catalog` on PostgreSQL through `pgtest` (`score_pins_test.go`, `score_overrides_test.go`, `score_tag_weights_test.go`), with `openCatalog`, `retain`, `setPin` and `tagWeights`. Existing tests stay unedited: a full namespace is saved as before.

Blocked by: None

Status: resolved

- [x] With `female:big hat` in the Panda catalog, `SetScorePin` for the tag `f:big hat` stores the pin of that catalog tag and returns it with the label `female:big hat`. The same holds for `F:Big_Hat`.
- [x] With `female:big hat` in the Panda catalog, `SetScoreOverride` for the tag `f:big hat` stores the override of that catalog tag, and the stored override has the label `female:big hat`.
- [x] Every short form of `gallerysearch.NamespaceShortForms` saves a pin for a catalog tag of its namespace (for example `a:x` for `artist:x`, `cos:x` for `cosplayer:x`).
- [x] A short form with a value that the catalog does not have under the full namespace returns `ErrTagNotFound` from both, and stores nothing.
- [x] `ScoreTagWeights` for `f:big hat`, with `female:big hat` in the catalog, answers `Known: false` for that entry.
- [x] `go vet ./...` and `go test ./...` pass, `gofmt -l .` prints nothing, and the existing tests of the package pass unedited.

## Resolution

`SetScorePin` and `SetScoreOverride` parse through the new `parseSavedScoreTarget` in `internal/collector/catalog/score_overrides.go`, which expands a tag namespace short form after `parseScoreTarget`; `ScoreTagWeights` keeps `parseScoreTarget` unexpanded, and tests are in `score_short_forms_test.go`.
