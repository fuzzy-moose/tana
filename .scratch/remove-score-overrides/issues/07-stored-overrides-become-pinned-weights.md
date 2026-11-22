# 07: Stored overrides become pinned weights

What to build: the first collector start with this version turns every stored score override into a pinned weight and drops the override table, and the listing's score grades follow the new pins from then on. Spec behaviours 14, 15 and 16 (the data); spec **Design → Conversion** and **Fit** 1.

The start-up refresh needs no change. Tests seed by SQL; the listing criterion needs only an active model row at threshold 0, no weights. `TestScoreModelsFromFavoritesMigrationDeletesEveryModel` seeds an override, which now converts: its pin count changes, and its count of the dropped table goes. `CONTEXT.md` and ADR 0019 get the current design.

Blocked by: 06

Status: resolved

- [x] After the migration, an "always hide" override of a tag, an uploader and a category each is a pin of -100 on the same value, and a "never hide" override a pin of +1000.
- [x] A value with an override and a pin has one pin, with the converted weight; other pins keep theirs. The table `panda_score_overrides` no longer exists.
- [x] With at least one override, the migration leaves `panda_favorite_scores` with empty `scores` and unchanged `model_id`, `query` and `categories`. Without one it leaves the row as it was.
- [x] With an active model after the migration, a gallery with a converted "never hide" and "always hide" value is listed, and one with only the "always hide" value is in the `hidden` state.
- [x] `grep -i override CONTEXT.md docs/adr/0019-collector-keeps-panda-seen-state.md` prints nothing.
- [x] `go vet ./...` and `go test ./...` pass, `gofmt -l .` prints nothing, and `go generate ./internal/collector/storage/` leaves no uncommitted change.

## Resolution

Migration `0020_score_overrides_to_pins.sql` turns each score override into a pin (-100 for "always hide", +1000 for "never hide", replacing an existing pin's weight), empties the stored held-out favorite scores when an override existed, and drops `panda_score_overrides`; new storage tests cover the conversion, the listing under an active model and the untouched favorite scores without overrides. `CONTEXT.md` and ADR 0019 describe pins only.
