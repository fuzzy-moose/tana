# Remove score overrides

## Problem

The score filter has two overlapping controls: pinned weights and score overrides ("Always hide" / "Never hide"). An override decides before the score, so the evaluation page, the listing and every measured number carry a second set of rules.

## Solution

The user controls the score filter with pinned weights only. A gallery is hidden when its Panda gallery score is below the score threshold, and for no other reason. Stored overrides become pinned weights once, and nothing of overrides stays on the pages, in the API or in the database.

## Fit

Fits, because pinned weights already do the work: the score adds every pin (`scoreSum`), and a pinned tag already gets its mark, colour and rank from `source: pin`. The feature removes code. No prefactor.

Kept, with the reason:

1. **Migrations are SQL files**, so the conversion cannot run the held-out calculation, which is Go. The migration empties the stored held-out favorite scores and keeps their model and scope; the refresh that every collector start already runs (`RefreshFavoriteScores`) fills them. Without the stored model that refresh would take the scope of the current counts. Unemptied scores would lack the converted favorites if that refresh fails.
2. **Many tests in catalog, storage and both httpapi packages put the score filter in force with an override**, because that needs no model. Those of behaviour that stays (behaviours 8 and 13) get an active model instead; only tests of the override rules go. This edits tests, so it is no prefactor.
3. **The evaluation page reads the override numbers of a preview unguarded**, so ticket 04 changes the collector and the page's share functions together, and the web stops calling the override routes (ticket 05) before they go (ticket 06).

## Behaviour

Terms: the *evaluation page* is `#/panda/score`, the *listing* is `#/panda`, a *pin* is a pinned weight.

Evaluation page:

1. The page has no "Overrides" section and no control that saves or removes a score override. The tab that held the section is named "Pins" and holds the "Pinned weights" section.
2. The weights table has no "Override" column. Each row, also the row of a typed tag that no listed weight has, keeps the pin field, "Pin", "Ignore" and, for a pinned value, "Remove pin".
3. The add form is named "Add pin". Its actions are "Pin" and "Ignore", and it pins the typed weight or 0. Its messages speak of the pin only.
4. Nothing sets a fixed weight in one click in place of "Always hide" or "Never hide".
5. The dialog "Turn off the score model?" says: "The model is deleted and can't be restored. The listing then hides nothing by score."
6. Every favorite in scope has a held-out favorite score. A threshold hides the share of them whose score is below it. The suggested threshold is the highest that hides at most 5 % of them.
7. A threshold hides the share of the newest catalog galleries whose score under that model is below it. The share hidden "now" is the one of the active model and its threshold, and 0 % without an active model.

Listing:

8. The score filter is in force only while a Panda score model is active. Without one the listing hides nothing by score, reports no score filter, gives no gallery a score and no tag a weight or mark, and "Hidden only" is empty. Pins and the author bonus stay stored and have no effect.
9. With an active model a gallery is hidden exactly when its Panda gallery score is below the score threshold. Pins add up with the other weights: no pin is absolute, and opposite pins on one gallery cancel.
10. The hover details name one reason for a hidden gallery: "Hidden: the score is below the threshold."
11. A card's tags are ranked by the absolute value of their effective weight, the strongest first, then the tags without a weight. A pinned tag shows the pin mark, else a favorite author's tag the star. Every tag takes its colour from its weight. No tag has an eye mark.
12. Score grades compare with the held-out favorite scores of all favorites in scope. A gallery with a +1000 pin gets the top score grade.
13. Unchanged with a model that hides: the filter applies after the search and the default filter, pages through several categories, follows provisional and then collected tags, keeps hidden galleries unseen, reports hidden unseen galleries on an empty first page of the unseen view, and a catch-up covers hidden galleries.

Stored overrides:

14. The first collector start with this version converts each stored score override into a pin on the same tag, uploader or category: "Always hide" becomes -100, "Never hide" becomes +1000. A pin that the value already has gets the converted weight. Other pins stay. Under an active model with an ordinary threshold, a gallery that carries a converted "Never hide" value and a converted "Always hide" value stays visible, as today.
15. After that start the stored held-out favorite scores equal a fresh calculation under the new pins.
16. No override is stored, and no route of the collector or of Tana saves, removes or lists one. Saving a pin answers as today, also for an invalid value, an unknown tag and a namespace short form.

## Design

**Flow.** Unchanged: the browser asks Tana, Tana asks the collector through `internal/collectorapi`, and `internal/collector/catalog` owns the score filter, the preview, the pins and the stored favorite scores. `httpapi/` of both services only loses routes, and behaviour 16 is their whole contract. Presentation stays in `web/src/panda/`.

**Catalog page** (collector → Tana → browser):

- `score_filter` is present only with an active model and then carries `threshold_tenths`.
- An item's `score` has `value`, `weights`, `favorites_above`, `grade` and `rule`. `rule` is `below_threshold` or absent. It has no `override`.
- A tag of `summary_tags` has `weight`, `source` and `favorite_author`, and no `override`.

**Preview** (`POST …/score/preview`):

- No `overrides`.
- `recent` is `total`, `candidate`, `active`. `candidate.held_out` is `total`, `scores`. Neither has `always_hidden` or `never_hidden`, and every counted gallery or favorite is in the histogram.
- A hidden share is the buckets below the threshold divided by `total`.

**Conversion.** The constants are -100 and +1000. The stored held-out favorite scores follow Fit 1.

**Risky choices.**

- Accepted by the brief: +1000 and -100 on one gallery give +900, so converted pins keep today's result only where no other strong weight interferes; a user with overrides and no active model sees every hidden gallery again; favorites with a converted pin score near the edge of the score window (±1000) and enter the suggested threshold, the score grade cut points and the histograms.
- If the start-up refresh fails, the listing shows no score grades until the next pin, bonus or model change, never stale ones.

**Test seams.**

- Listing and preview: the tests of `internal/collector/catalog` on PostgreSQL.
- Conversion: the migration tests of `internal/collector/storage`, which seed rows before the new migration runs.
- Web: the tests beside the components in `web/src/panda/`.
- Removed behaviour gets no new test (`AGENTS.md`). Its absence shows in the types, the build and the grep criteria of the tickets.

## Assumptions

- **`rule` stays a string with the one value `below_threshold`.** Rejected: a boolean in its place. The string keeps the wire and the tests of the threshold rule as they are.
- **The tab is named "Pins".** Rejected: "Pinned weights", which would repeat the heading of its only section.
- **`total` stays in `held_out`**, where it now equals the sum of the histogram. Rejected: dropping it. The page divides by it.
- **This spec reverses rules 8, 11 and 27 of `.scratch/panda-score-pins/spec.md`.** The old specs stay unedited.
- **No ADR decision changes.** ADR 0019 and `CONTEXT.md` get the current design in ticket 07. `CONTEXT.md` loses "Score override"; no term is missing.
- **Both services are updated together.** They are deployed independently. Rejected: a wire that an older page or Tana still reads.

## Out of scope

- A one-click "hide" or "keep" control, and any pin that is absolute.
- Another score window, histogram or score grade rule for extreme pins.
- The pin editor of the local gallery detail view.

## Manual checks

- **Weights table and add form.** On `#/panda/score` at a narrow and a wide window: rows stay on one line without the "Override" column, and the form reads well with two actions. Tune `--score-label-min-width` and `--score-scroll-height` (`PandaScorePage.css`).
- **Conversion on real data.** Start the collector on a copy of the real database. Good: "Pinned weights" lists a -100 or +1000 pin for each former override, "Hidden only" holds the galleries it held before (with an active model), converted tags show the pin mark on the cards, and cards show score grades. Tune: -100 and +1000, the brief's values.
- **Threshold section with extreme pins.** Good: the suggested threshold and the two shares still read sensibly with scores near ±1000.
