# 03: The "count Panda favorites" setting is gone

What to build: the score page has no switch for Panda favorites, and no request, model or answer carries the setting. Until ticket 04 replaces the calculation, the preview counts every Panda favorite without a verdict as "Interesting" evidence, as it does today with the switch on. The browser sends nothing that steers the calculation; Tana adds the default filter.

This is spec behaviours 17 and 30. Layers, as fixed in spec **Design → `internal/collectorapi`**, **Routes**, **Collector schema** and **Web (Score request)**:

- `internal/collector/storage/migrations`: the next free number (0011) drops `panda_score_models.favorites`. Stored models stay; ticket 04 deletes them. Test code that inserts model rows (ticket 02) stops inserting the column.
- `internal/collectorapi/catalog_score.go`: `ScorePreviewOptions` holds only `DefaultQuery` and `DefaultCategories`; `ScorePreview.Favorites`, `ScoreCandidate.Favorites` and `ScoreActive.Favorites` go.
- `internal/collector/catalog`: `PreviewScoreModel` always reads the favorites; the active model and the stored candidate hold no favorites setting. Ticket 04 replaces this calculation, so no new test pins that favorites always count.
- `internal/local/httpapi/catalog_score.go`: `HandleCollectorScorePreview` decodes an empty object and sends the default filter.
- `web/src/panda/score.ts`, `PandaScorePage.tsx`: `previewPandaScore(signal)` posts `{}`; the switch, its handler and the `favorites` fields of the types go; the active model reads "Calculated <time>, threshold <t>."

Tests of removed behaviour are deleted, not ported, and no new test pins that something is gone (spec **Design → Test seams**): the collector tests of the switch (`TestPreviewCountsFavoritesOnlyWithTheSwitch`, `TestPreviewFavoritesFollowTheActiveModel`, the "without favorites" variants of the author bonus tests) and the web tests of the switch go. Tests that used the switch only to cause a second calculation keep their point with another cause (an override change or Retry), or go when an existing test already covers it. The request cases of `TestScorePreviewValidation` and `TestLocalScoreModel` lose their `favorites` field.

Test seams: `internal/collector/catalog` (`PreviewScoreModel`), `internal/collector/httpapi/catalog_test.go` (`TestScorePreviewValidation`), `internal/local/httpapi/catalog_test.go` (`TestLocalScoreModel`, which captures the body sent to the collector), the migrated schema that every collector test runs on, and `web/src/panda/PandaScorePage.test.tsx`.

Blocked by: None

Status: resolved

- [x] The collector answers `POST /api/catalog/score/preview` with 200 for `{"default_query":"","default_categories":[]}`, and still with `invalid_category` and `invalid_query` for an invalid default filter.
- [x] On the migrated schema the catalog listing, apply, a threshold change and turn-off work as before: the tests of ticket 02 pass, changed only in the columns that their stored rows carry.
- [x] Tana answers `POST /api/collector/catalog/score/preview` with body `{}` by sending the collector exactly the keys `default_query` and `default_categories`, filled from the saved default filter.
- [x] Every calculation that the score page starts (visit, Retry, after an override, pin, bonus, apply, threshold or turn-off change) posts the body `{}`. (Ticket 08 adds `candidate_id` to some of them.)
- [x] The active model section reads "Calculated <time>, threshold 1.5." for a threshold of 15 tenths.
- [x] A threshold typed into the candidate field stays through a new calculation, as before.
- [x] Removal: `ScorePreviewOptions.Favorites`, `ScorePreview.Favorites`, `ScoreCandidate.Favorites`, `ScoreActive.Favorites`, the column `panda_score_models.favorites` (dropped by the new migration), the `favorites` fields of `ScorePreview`, `ScoreCandidate` and `ActiveScoreModel` in `score.ts`, and the switch "Count Panda favorites as Interesting" no longer exist. A search for `favorites` finds no request field in `internal/local/httpapi/catalog_score.go` and no argument of `previewPandaScore`.
- [x] The `go` and `frontend` checks of the green check pass, and `gofmt -l .` prints nothing.

## Resolution

Migration 0011 drops `panda_score_models.favorites`; `collectorapi`, the collector preview (always reads favorites), the local preview route (decodes `{}`), `score.ts` and `PandaScorePage.tsx` lose the favorites setting and switch; switch tests are deleted or recast with an override change as the cause of a new calculation.
