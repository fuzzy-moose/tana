# 06: No override API

What to build: neither the collector nor Tana offers a way to save, remove or list a score override, and no Go code reads or writes one. Saving a pin answers as before. The API part of spec behaviour 16.

What goes: the collector's `POST /api/catalog/score/overrides` and `DELETE /api/catalog/score/overrides/{id}`, Tana's two routes under `/api/collector/catalog/score/overrides`, the `collectorapi` client methods and types for them, the rules `always_hide` and `never_hide`, the error code `invalid_override`, `overrides` of a preview, and the tests of the override store and routes. `parseScoreTarget`, `parseSavedScoreTarget` and `ErrTagNotFound` stay for the pins and the tag weight lookup. Comments that describe a pin or a weight by an override get the current design. The table `panda_score_overrides` stays, unread, until ticket 07.

Blocked by: 01, 05

Status: resolved

- [x] The key list that `TestPreviewJSON` pins for a preview has no `overrides`.
- [x] Saving a pin through both services answers `invalid_pin` (400) for an invalid value and `tag_not_found` (404) for an unknown tag and expands a namespace short form. `ScoreTagWeights` still does not expand one.
- [x] `grep -rniE "override|(always|never)[_ ]?hid" internal/collector/catalog internal/collectorapi internal/collector/httpapi internal/local/httpapi` prints nothing.
- [x] `grep -rn "panda_score_overrides" internal --include=*.go` finds only test files of `internal/collector/storage`.
- [x] `go vet ./...` and `go test ./...` pass, and `gofmt -l .` prints nothing.

## Resolution

Removed the override routes of both services, the `collectorapi` override client methods, `ScoreOverride`, the rules `always_hide`/`never_hide`, `invalid_override` and a preview's `overrides`; `score_overrides.go` became `score_target.go` with only `parseScoreTarget`, `parseSavedScoreTarget` and `ErrTagNotFound`. Override tests went; `TestPreviewJSON` now pins the preview's keys and `TestLocalScorePins` saves a namespace short form through Tana and the collector.
