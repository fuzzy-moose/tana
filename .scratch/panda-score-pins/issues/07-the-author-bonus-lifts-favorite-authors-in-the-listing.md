# 07: The author bonus lifts favorite authors in the Panda listing

What to build: Tana can tell the collector one author bonus, a number above 0, and turn it off again. While it is on and a Panda score model is active, the tag of each favorite author weighs at least the bonus in the Panda gallery score: the larger of its calculated weight and the bonus, and the bonus when it has no calculated weight. A pin on the tag wins. The next catalog page after a change hides, shows, scores and ranks by it, and says for each summary tag and score weight when the bonus gives the weight. With an active model, a catalog page also says which summary tags are favorite authors, with the bonus on or off. The bonus control comes in ticket 13, and the marks on the cards in ticket 10.

This is spec behaviours 18 to 22, 52 and 55 (bonus), the collector side of behaviours 26 and 28, and the `favorite_author` of behaviour 54. Its layers are specified in spec **Design**:

- Migration `internal/collector/storage/migrations/0010_panda_score_settings.sql`: the table `panda_score_settings` of **Design → Collector schema** with its one row, and a comment on its purpose.
- `internal/collectorapi`: the constant `ScoreSourceAuthorBonus`, `Client.SetAuthorBonus` and `CatalogTag.FavoriteAuthor` (`json:"favorite_author,omitempty"`).
- `internal/collector/catalog`: **Design → The author bonus (ticket 07)** and the favorite author part of **Design → The weight rule** in the SQL builder. `ErrInvalidAuthorBonus`, `SetAuthorBonus` (with the bound `maxScoreWeight` of ticket 03), the bonus read by the score filter with the pins, and the builder's favorite author part with the source `author_bonus`. The builder probes `favorite_authors` by `tag_id` and never reads `favorites`. The source is `author_bonus` only when the bonus gives the weight: the tag has no pin, is a favorite author, and has no calculated weight or one below the bonus. `belowThreshold` and `inForce` stay as they are, so the bonus has no effect without an active model.
- `summarizeTags` sets `FavoriteAuthor` on each tag that `favorite_authors` holds, while a model is active. The flag is a fact of its own: it does not depend on the bonus, on a pin or on an override of the tag. Without an active model no tag has it.
- `internal/collector/httpapi`: `PUT /api/catalog/score/author-bonus` with `{"bonus": number or null}` (204; an absent `bonus` counts as null), and the case `ErrInvalidAuthorBonus` → 400 `invalid_author_bonus` in `catalogError`.
- `internal/local/httpapi/catalog_score.go`: `HandleCollectorSetAuthorBonus`, registered as `PUT /api/collector/catalog/score/author-bonus`, like `HandleCollectorSetScoreThreshold`. `writeCatalogCollectorError` passes 400 `invalid_author_bonus` through.
- Docs:
  - `CONTEXT.md`, the entry **Favorite author** gains: "The author bonus is the one weight that the user sets as the minimum for the tag of every favorite author. It is off until set, and a pinned weight on the tag wins."
  - `CONTEXT.md`, the entry **Panda gallery score** becomes: "The sum of the weights of a gallery's tags, uploader, and category under the active Panda score model. A pinned weight replaces the model's weight of its value, and the tag of a favorite author weighs at least the author bonus."
  - `docs/adr/0019-collector-keeps-panda-seen-state.md`: one sentence that the collector keeps the author bonus with the pinned weights.

Migration 0010 follows migration 0009 of ticket 06. This ticket also reads the table of ticket 06 and extends the builder and the `Source` fields of ticket 04.

Hint: while the bonus is off, the builder can leave the favorite author probe out of the weight expression. The `FavoriteAuthor` flag of `summarizeTags` needs its probe with the bonus on and off.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `seedVerdicts`, `scorePage`, `itemByID`, `seedScoreEvidence`, `applyCandidate`, `previewScore`, `weightOf` and `assertSummaryTags`. A test makes a tag a favorite author by inserting its row into `favorite_authors` directly. A validation table in `internal/collector/httpapi/catalog_test.go`, like `TestScoreModelValidation`. A local test in `internal/local/httpapi/catalog_test.go` through the real collector handler. Existing assertions stay unchanged.

Hints (optional): with `seedScoreEvidence` and `applyCandidate(t, s, -3)`, `artist:a` has a calculated weight of about -0.47 and `female:glasses` of about -0.69, so a gallery whose only tag is `artist:a` is hidden, and a bonus of 2 lists it with the score 2. A tag that only one retained gallery carries has no calculated weight. Three more "Interesting" evidence galleries with one artist tag give that tag a positive weight.

Blocked by: 04, 06

Status: resolved

- [x] `SetAuthorBonus` stores a number above 0, and `nil` turns the bonus off. A number that is 0, negative, NaN, infinite or above 1e300 returns `ErrInvalidAuthorBonus` and leaves the stored bonus as it was.
- [x] With an active model and the bonus on, the tag of a favorite author weighs at least the bonus in the next catalog page, with no preview and no apply in between: a tag with a calculated weight below the bonus and a tag without a calculated weight both count with the bonus. An `artist` tag that is no favorite author gets no bonus. After the bonus is turned off, every tag counts as before.
- [x] Where the bonus gives the weight, the tag's entry in `Score.Weights` and in `SummaryTags` has the bonus as weight and the `Source` `"author_bonus"`. A favorite author whose calculated weight is at or above the bonus keeps that weight with an empty `Source`.
- [x] A pin wins: a pinned favorite author's tag counts with its pin and has the `Source` `"pin"`, with the bonus on.
- [x] The bonus applies under a model that counts Panda favorites as "Interesting" and under one that does not.
- [x] Without an active model the bonus has no effect: with the bonus on and no override, `List` returns a nil `ScoreFilter`, no item has a `Score`, and no summary tag has a `Weight`, a `Source` or `FavoriteAuthor`.
- [x] With an active model, each summary tag that is a favorite author has `FavoriteAuthor` true: with the bonus on and with it off, with a pin, and with a score override. No other tag has it.
- [x] Collector `PUT /api/catalog/score/author-bonus` returns 204 for a number above 0, for `{"bonus":null}` and for `{}`. A number that is not above 0 gives 400 `invalid_author_bonus`, and a body with unknown fields 400 `invalid_json`.
- [x] Through the real collector handler, local `PUT /api/collector/catalog/score/author-bonus` returns 204 and the collector holds the bonus, and 400 `invalid_author_bonus` passes through. Without a collector the route answers 503 `collector_not_configured`, and with a rejected token 502 `collector_unauthorized`. A local catalog page with an active model has `"favorite_author":true` on the `summary_tags` entry of a favorite author's tag, and no `favorite_author` key on another tag.
- [x] `CONTEXT.md` has the extended **Favorite author** entry and the changed **Panda gallery score** entry, and ADR 0019 names the author bonus.
- [x] `go generate ./internal/local/storage/ ./internal/collector/storage/` leaves no uncommitted change. `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

Migration 0010 adds `panda_score_settings` with the author bonus; `catalog.SetAuthorBonus` stores it, the score filter reads it with the pins, and the SQL weight builder (`weightSQL`, `sourceSQL` in `score_filter.go`) lifts favorite authors' tags to the bonus with the source `author_bonus`, while `summarizeTags` marks `FavoriteAuthor` under an active model. Collector `PUT /api/catalog/score/author-bonus`, local `PUT /api/collector/catalog/score/author-bonus`, `collectorapi.Client.SetAuthorBonus`, `CatalogTag.FavoriteAuthor`, CONTEXT.md and ADR 0019 follow.
