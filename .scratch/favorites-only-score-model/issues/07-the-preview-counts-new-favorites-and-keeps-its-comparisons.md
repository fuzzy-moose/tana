# 07: The preview counts new favorites and keeps its comparisons

What to build: with an active model, the score page says how many favorites in scope are new since that model was calculated, so the user sees whether a new calculation is worth it. A favorite is new when the collector first observed its gallery as a favorite after the calculation. The page already has the row and shows it as soon as the collector sends a number (ticket 05), so this ticket changes the collector only. The parts of the preview that sit beside the candidate work on the favorites model as they did before: the description of the active model, each weight's value in the active model, the pins with the candidate's weight of their value, and the favorite authors with their effective weight. Ticket 04 removed their verdict-built tests; this ticket pins them again.

This is spec behaviour 21, the collector side of the "new favorites" row of 20, and the preview part of 14. Fixed in spec **Design → Calculation and measurement → Evidence numbers** and **Collector schema**:

- Migration, next free number (0013): adds `favorite_observations.observed_at TIMESTAMPTZ NOT NULL DEFAULT now()`. `ObserveFavorite` and `SeedFavoriteObservations` insert only `gallery_id`, so the `favorites` package needs no change. Run `go generate ./internal/collector/storage/` and commit any `dbgen` change.
- `internal/collector/catalog`: `Evidence.NewFavorites` is the number of favorites in scope whose `favorite_observations.observed_at` is later than the active model's `calculated_at`, nil without an active model. `Active`, `ActiveWeight`, `Pins[].ModelWeight`, `AuthorBonus`, `FavoriteAuthorCount` and `FavoriteAuthors` need tests and, as far as ticket 04 left them working, no code.
- No route and no payload shape changes.

Test seams: `PreviewScoreModel` in `internal/collector/catalog` with favorites as in ticket 04, rows in `favorite_observations` with chosen `observed_at` times, a stored active model (ticket 02) or a real apply, pins, the author bonus and rows in `favorite_authors`; a check on the migrated schema.

Hints (optional): a test sets `observed_at` relative to the stored `calculated_at` of the active model, so it needs no sleep. The candidate lists only its 500 strongest weights; a pin on a weaker value still gets its `ModelWeight`.

Blocked by: 04

Status: resolved

- [x] `Evidence.NewFavorites` is nil without an active model. With one it counts the favorites in scope that were observed after the model's calculation: a favorite observed before the calculation does not count even when its `favorites` row is newer, a favorite outside the scope does not count, a favorite without an observation row does not count, and a gallery in two collections counts once.
- [x] After the migration every `favorite_observations` row has an `observed_at`, and a row inserted with only its `gallery_id`, as the favorite sync inserts it, gets the current time.
- [x] With an active model, `Active` holds its calculation time and threshold, each listed weight carries the value's weight in the active model as `ActiveWeight`, and a value that the active model does not weigh has a nil `ActiveWeight`. Without an active model `Active` is nil and `Recent.Active` is nil.
- [x] The preview lists every pin by ID, and a pin carries the candidate's weight of its value as `ModelWeight`, also when that weight is not among the 500 listed. A pin on a value that the candidate does not weigh, and every pin without a candidate, has none.
- [x] The preview carries the author bonus, the number of favorite authors, and at most 500 authors with the most favorites, each with its effective weight under the candidate: its pin, else the larger of its candidate weight and the bonus, else its candidate weight, nil without one. Without a candidate the authors are still listed.
- [x] Through the real collector handler, Tana's preview answer holds `evidence.new_favorites` as the collector counted it.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean.

## Resolution

Migration 0013 adds `favorite_observations.observed_at`; `PreviewScoreModel` fills `Evidence.NewFavorites` with the favorites in scope observed after the active model was calculated. New tests pin the migration, the new-favorites count, the active-model comparison, pin model weights beyond the 500 listed and favorite author weights under a candidate (`score_comparisons_test.go`), and `new_favorites` through the local route.
