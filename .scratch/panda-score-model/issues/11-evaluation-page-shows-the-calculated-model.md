# 11: Evaluation page shows the calculated model

What to build: the user opens "Score model" from the Options menu of the Panda listing and gets a page that calculates a new model from the current evidence. It shows the evidence counts, a switch that calculates with or without Panda favorites, the table of weights beside the active model's weights, a warning for few verdicts, and what to do when no model is possible. If the collector can't be reached, it shows the error and a retry. Threshold, apply and the active model's controls come in ticket 12, and overrides in ticket 13.

This is spec behaviours 13 to 16, 22, 23 and 46. Its parts are specified in spec **Design → Web**:

- `web/src/navigation.ts`: the route kind `'panda-score'` for the path `/panda/score`. `web/src/App.tsx` renders `PandaScorePage` for it and keeps the side navigation's Panda entry active, as for `'panda-lookup'`.
- `web/src/panda/PandaCatalog.tsx`: the Options menu gains the item "Score model", a link to `#/panda/score`, beside "Look up gallery".
- `web/src/panda/score.ts`: the preview types as in the JSON of spec **Design → `internal/collectorapi`** (declare the whole shape now; `held_out`, `suggested_threshold_tenths` and `recent` are used in ticket 12), and `previewPandaScore(favorites: boolean | null, signal: AbortSignal)`, which posts `{"favorites": ...}` to `/api/collector/catalog/score/preview` with `collectorMessages` plus messages for `invalid_query` and `invalid_category` ("The default filter is invalid. Correct it in the Panda listing.").
- `web/src/panda/PandaScorePage.tsx`, laid out like `PandaLookupPage` (back link "Panda catalog", heading "Score model", one introducing sentence):
  - On mount it requests the preview with `favorites: null`. While the first request runs it shows "Calculating…". A later request keeps the last result on screen with a spinner.
  - A failed request shows a red `Callout` with `role="alert"`, the message, and "Retry", which requests again. A result that is already on screen stays.
  - Section "Evidence": the counts with their labels ("Interesting verdicts", "Not for me verdicts", "Neutral galleries", "Counted favorites"), "Last Panda favorite sync" with the local time or "never", and, only with an active model, "Verdicts since the active model was calculated".
  - A `Switch` labelled "Count Panda favorites as Interesting" shows the preview's `favorites`. Changing it requests the preview with that value.
  - Section "Active model" in this ticket: "No model is active." or the active model's calculation time, "with favorites" or "without favorites", and its threshold with one decimal.
  - Section "New model". With `few_verdicts`, an amber `Callout`: "Few verdicts. Weights and error rates from so little evidence are unreliable." Without a candidate, no table, and one message by `no_model`:
    - `no_verdicts`: "No verdicts yet. Mark galleries as Interesting or Not for me with the two controls on each card of the Panda listing, then come back."
    - `no_not_for_me`: "No "Not for me" verdicts yet. A model needs at least one: mark galleries you would skip as Not for me on their cards in the Panda listing."
    - `no_acceptable`: "No acceptable galleries yet. A model needs at least one gallery marked Interesting, or one gallery that you passed in the Panda listing without a verdict."
  - The weights table: columns "Tag, uploader or category", "Weight", "Interesting", "Not for me", "Neutral", "Active weight". Rows in the order the collector sent. A tag shows its label, an uploader "Uploader <label>", a category "Category <label>". Weights show a sign and two decimals, and a missing active weight shows "–". 50 rows show at first, and "Show more" adds 50 while more are loaded. A line above says "<shown> of <weight_count> weights, strongest first."

Test seams: a new `web/src/panda/PandaScorePage.test.tsx` that renders the page (or `<App />` at `#/panda/score`) with a stubbed `fetch`, as `PandaLookup.test.tsx` does. The Options menu link gets one case, in the new file or in `PandaCatalog.test.tsx`. Existing assertions stay unchanged; new cases may be added to existing test files. Layout is a manual check.

Blocked by: 09

Status: resolved

- [x] At `#/panda/score` the page with the heading "Score model" renders, posts `{"favorites":null}` to `/api/collector/catalog/score/preview`, and the side navigation marks Panda as active. The Options menu of the Panda listing has a link "Score model" to `#/panda/score`.
- [x] The four evidence counts of the preview show with their labels. `favorites_synced_at` null shows "never". "Verdicts since the active model was calculated" shows with its number when `active` is set, and is absent when `active` is null.
- [x] The switch is on when the preview's `favorites` is true. Clicking it posts `{"favorites":false}`, and the counts and the table of the new response replace the old ones.
- [x] The table shows the weights in the order of the response, each with its formatted weight, its three counts and its active weight or "–", and uploaders and categories with their prefix.
- [x] With 120 weights in the response, 50 rows show, "Show more" brings 100, a second click 120, and the button then is gone. The line above names the shown number and `weight_count`.
- [x] With `few_verdicts` true the warning shows and the table still shows.
- [x] With `no_model` `no_verdicts` the message that starts "No verdicts yet." shows, with `no_not_for_me` the one that starts "No "Not for me" verdicts yet.", and with `no_acceptable` the one that starts "No acceptable galleries yet." Each case shows only its own message and no table, and an active model's summary still shows.
- [x] A 502 `collector_unavailable` on the first request shows the error in an alert with "Retry" and no evidence section. "Retry" posts the preview again and shows the result.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

Added the `'panda-score'` route at `/panda/score` (`web/src/navigation.ts`, `web/src/App.tsx`), the "Score model" Options menu link in `PandaCatalog.tsx`, the preview types and `previewPandaScore` in `web/src/panda/score.ts`, and `web/src/panda/PandaScorePage.tsx` with evidence, favorites switch, active model summary, new-model warnings and messages, and the paged weights table; covered by `PandaScorePage.test.tsx` and one case in `PandaCatalog.test.tsx`.
