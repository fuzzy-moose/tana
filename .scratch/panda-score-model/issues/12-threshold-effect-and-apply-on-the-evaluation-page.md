# 12: Threshold, effect and apply on the evaluation page

What to build: on the evaluation page the user sets a threshold for the new model and sees at once what it would hide: the share of "Not for me", "Interesting" and neutral evidence galleries, the two error rates, and the share of recent catalog galleries, beside the share the listing hides now. The threshold stays where the user put it while the favorites switch or an override brings a new calculation. One button applies the model with that threshold. For the active model the user can save another threshold or turn the model off. A failure changes nothing and shows a message.

This is spec behaviours 17 to 21, 47, 49 and 50 (the web side), and the web side of behaviour 52. Its parts are specified in spec **Design → Web** and the histogram rules of **Design → `internal/collectorapi`**:

- `web/src/panda/score.ts`:
  - `hiddenShare(effect, thresholdTenths)`: `(always_hidden + the counts of all buckets below the threshold) / total`, and 0 for `total` 0. Bucket `i` is below the threshold when `first_bucket + i < thresholdTenths`.
  - `recentHiddenShare(recent, histogram | null, thresholdTenths)`: `(not_for_me + always_hidden + the histogram's counts below the threshold) / total`, and 0 for `total` 0. With a null histogram only the first two count.
  - `currentHiddenShare(preview)`: the share of the recent sample that the listing hides now.
    - `active` null and `overrides` empty: 0. The score filter is not in force, so the listing hides nothing, whatever `recent.not_for_me` says.
    - `active` null and at least one override: `(not_for_me + always_hidden) / total`.
    - `active` set: `recentHiddenShare(recent, recent.active, active.threshold_tenths)`.
  - `applyPandaScore(candidateID, thresholdTenths, signal)` (`POST .../score/apply`), `savePandaScoreThreshold(thresholdTenths, signal)` (`PUT .../score/threshold`), `turnOffPandaScore(signal)` (`DELETE .../score/model`), with `collectorMessages` plus: `score_candidate_not_found` → "This calculation is no longer available. Calculate again.", `score_model_not_active` → "No score model is active any more.", `invalid_threshold` → "Enter a threshold between -100 and 100."
- `web/src/panda/PandaScorePage.tsx`:
  - Threshold state. The page keeps two values in tenths (the entered number times 10, rounded): the threshold for the new model and the threshold for the active model. Each gets its start value from the first preview that can supply it, and again from the first preview after a successful apply, threshold save or turn-off. The start value for the new model is `active.threshold_tenths` when a model is active, else `candidate.suggested_threshold_tenths`. The start value for the active model is `active.threshold_tenths`. No other preview changes these values: after the favorites switch, an override change (ticket 13) or a retry, each field still shows what the user entered.
  - Section "New model", above the weights table and only with a candidate: a number field "Threshold" with `step="0.1"`, and beside it "Suggested: <candidate.suggested_threshold_tenths / 10 with one decimal>" for the candidate on screen.
  - Beside the field, recalculated on every change of the field and without a request: "Hides <x> % of Not for me", "<y> % of Interesting", "<z> % of neutral" from `candidate.held_out`, and "Hides <r> % of the newest <recent.total> catalog galleries (now: <n> %)", with `r` from `recentHiddenShare(recent, recent.candidate, the field's value)` and `n` from `currentHiddenShare`.
  - "Error rates, measured on galleries the model did not learn from": "Interesting hidden: <y> %" and "Not for me shown: <100 − x> %".
  - A button "Apply model and threshold" calls `applyPandaScore` with the `id` of the candidate on screen and the field's value. On success the page shows "Model applied." and requests the preview again. On failure it shows "Could not apply the model." with the error, and requests nothing. An empty or invalid field disables the button.
  - Section "Active model", with an active model: a number field "Active threshold", the line "Hides <a> % of the newest <recent.total> catalog galleries" from `recentHiddenShare(recent, recent.active, the field's value)`, a button "Save threshold" (`savePandaScoreThreshold`), and a button "Turn off". "Turn off" opens an `AlertDialog`: "Turn off the score model? The model is deleted and can't be restored. Overrides keep working." with "Cancel" and "Turn off" (`turnOffPandaScore`). After each success the page requests the preview again. A failure shows "Could not save the threshold." or "Could not turn off the model." with the error.
  - Percentages show as whole numbers. One `useMutation` serves the three changes, so only one runs at a time.

Test seams: table tests for the three share functions in a test file beside `score.ts` or in `PandaScorePage.test.tsx`. Page tests in `PandaScorePage.test.tsx` (the file of ticket 11) with a stubbed `fetch` and a preview fixture with small histograms. Existing assertions stay unchanged; new cases may be added to existing test files. The feel of stepping the threshold and of comparing calculations is a manual check.

Blocked by: 08, 11

Status: resolved

- [x] `hiddenShare` for `{total: 10, always_hidden: 1, never_hidden: 1, scores: {first_bucket: -2, counts: [2, 2, 4]}}` is 0.1 at a threshold of -2 tenths, 0.3 at -1, 0.5 at 0 and 0.9 at 1 and above, and 0 for `total` 0. `recentHiddenShare` adds `not_for_me` and `always_hidden` to the buckets below the threshold, and counts only those two for a null histogram.
- [x] `currentHiddenShare` is 0 for a preview with `active` null and no overrides, also when `recent.not_for_me` and `recent.always_hidden` are not 0. With `active` null and one override it is `(not_for_me + always_hidden) / total`. With an active model it is the recent share under `recent.active` at the active threshold.
- [x] A preview with no active model and no overrides shows "now: 0 %" on the page, although the recent sample has "Not for me" galleries, and the candidate's own "Hides <r> %" still counts them.
- [x] With an active model the threshold field starts at the active threshold, and without one at the suggested threshold, each shown with one decimal. "Suggested:" names the suggested threshold of the candidate on screen in both cases.
- [x] Changing the threshold field changes the three evidence shares, the two error rates and the recent share to the values the fixture's histograms give, and sends no request.
- [x] After the user entered a threshold and the favorites switch brought a preview with another candidate (another `id`, another `suggested_threshold_tenths`), the field still shows the entered value, the shares are those of the new candidate's histograms at that value, and "Suggested:" shows the new candidate's value.
- [x] "Apply model and threshold" posts `{"candidate_id": <the id of the candidate on screen>, "threshold_tenths": <the field's value in tenths>}` to `/api/collector/catalog/score/apply`: after the favorites switch, the new candidate's `id`. After a 204 the page posts the preview again and shows "Model applied."
- [x] After a successful apply, threshold save or turn-off, both fields start again from the next preview: at the new active threshold, or, without an active model, the new-model field at the suggested threshold.
- [x] After a failed apply (502) the page shows "Could not apply the model." with the error, posts no preview, still shows the candidate, and the field keeps its value. A 409 `score_candidate_not_found` shows "This calculation is no longer available. Calculate again."
- [x] Without a candidate the page shows no threshold field for a new model and no apply button. With `few_verdicts` true the apply button is enabled.
- [x] "Save threshold" sends `PUT /api/collector/catalog/score/threshold` with the field's value in tenths, and the page then requests the preview again. A failure shows "Could not save the threshold." and requests nothing.
- [x] The line under "Active threshold" follows the field from `recent.active`, without a request.
- [x] "Turn off" opens the confirmation and sends nothing. "Cancel" closes it without a request. Confirming sends `DELETE /api/collector/catalog/score/model`, and the page then requests the preview again. A failure shows "Could not turn off the model." with the error.
- [x] Without an active model the page shows no "Active threshold" field, no "Save threshold" and no "Turn off".
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`web/src/panda/score.ts` gained `hiddenShare`, `recentHiddenShare`, `currentHiddenShare` (table tests in `score.test.ts`) and the apply, threshold and turn-off requests with their messages; `PandaScorePage.tsx` gained the two threshold fields (restarted only after a successful change), the live effect and error rates, "Apply model and threshold", "Save threshold" and "Turn off" behind one `useMutation`, with page tests in `PandaScorePage.test.tsx`. The favorites switch is locked while a change runs, and "Save threshold" and "Turn off" while a calculation runs, so a change's reload never drops a newer favorites setting.
