# 11: Pinned weights in the weights table and in a list on the evaluation page

What to build: on the evaluation page the user pins the weight of a tag, uploader or category from its row of the weights table. The row shows the calculated weight and, in a number field, the pin. "Pin" saves the field's number, "Ignore" pins the value to 0, and "Remove pin" removes the pin. A section "Pinned weights" lists every pin with "Remove", and says so when no model is active, because pins have no effect then. A pin shows at once in the row and in the list. The page then calculates again in the background, and the pin controls stay usable meanwhile, so several pins in a row do not wait for one another. The form for values that are not in the table comes in ticket 12.

This is spec behaviours 31, 32 and 34, and the pin side of behaviours 39 and 40. Its parts are specified in spec **Design → Web**:

- `web/src/panda/score.ts`: `ScorePin`, `ScorePreview.pins`, `savePandaScorePin` (`POST /api/collector/catalog/score/pins` with `{"kind","value","weight"}`, answers the stored pin) and `removePandaScorePin` (`DELETE /api/collector/catalog/score/pins/{id}`), with `collectorMessages` plus `tag_not_found` → "No gallery in the Panda catalog has this tag." and `invalid_pin` → "Enter a tag as namespace:value, an uploader name, or a Panda gallery category."
- `web/src/panda/PandaScorePage.tsx`, the items *Pin state*, *Pin changes*, *Pin controls*, *Weights table* and *Section "Pinned weights"* of **Evaluation page**:
  - The page holds the pins in its own state. Each preview replaces them with `preview.pins`. The answer of a saved pin replaces the pin of the same kind and label or is added, and a removed pin leaves the list. The rows and the list read this state.
  - One component holds the pin controls of a value: a number field named "Pinned weight of <label>" (the label as `weightLabel` gives it, `step="any"`), "Pin" (disabled while the field holds no number), "Ignore", and "Remove pin" only for a pinned value. Enter in the field does what "Pin" does. The field shows the pin's number (`String(weight)`) whenever the pin changes, and is empty without a pin. Ticket 13 uses the component again.
  - The weights table gets a column "Pin" after the column "Override". The first six columns stay as they are. A row is pinned when the pin state holds a pin with the row's kind and label. A row saves with its kind and its label as the value.
  - The section "Pinned weights" comes after "Overrides" and shows also without a candidate and without an active model. Without an active model it starts with "No score model is active: pinned weights have no effect." Then the list of all pins, each as `weightLabel` and `formatWeight` of its number with a button "Remove", or "No pinned weights." The list is a list, not a table: the page keeps one table.
  - Pin changes run through the page's one `useMutation`. The pin controls are disabled only while a change is being saved, and stay enabled while a calculation runs. All other controls keep today's rule (disabled during a calculation and during a change).
  - After a successful pin change the page updates the pin state, then calculates again with the favorites value of its latest preview request. This aborts a calculation in progress, whose answer is never shown. A pin change leaves the threshold fields as they are and never alters whether the next preview starts them again.
  - A failed pin change shows "Could not change the pin." with the error, in the "New model" section for a row and in the "Pinned weights" section for the list. It changes no state and requests no preview.

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`. Its fixtures `preview()` and `measured()` gain `pins: []`. Existing assertions stay unchanged: `rows()` reads the first six cells of a row, `screen.getByRole('table')` needs the page to have one table, and the "Overrides" section keeps its own "Remove" buttons. Enter in the field, the width of the table with the new column, and pins in a row against a real collector are not tested: the last two are manual checks (spec **Manual checks**).

Blocked by: None

Status: resolved

- [x] A row of the weights table shows the calculated weight in its "Weight" cell and the pin in its pin field: the field holds the pin's number and the row has "Remove pin" for a pinned value, and the field is empty, "Pin" is disabled and there is no "Remove pin" for a value without a pin.
- [x] "Pin" posts the row's kind, its label as the value and the field's number to `/api/collector/catalog/score/pins`. "Ignore" posts the weight 0. "Remove pin" on a row and "Remove" in the list send `DELETE /api/collector/catalog/score/pins/{id}` with the pin's ID.
- [x] A saved pin shows in its row and in the "Pinned weights" list, and a removed pin leaves both, before the calculation that follows has answered. The page then requests the preview again with the favorites value of its latest preview request, and a threshold that the user entered stays in its field.
- [x] While the calculation after a pin change runs, the pin controls are enabled and "Apply model and threshold" is disabled. A second pin saved in that time is sent at once, and the page then shows the numbers of the newest calculation, never those of the replaced one.
- [x] The "Pinned weights" section lists every pin of the preview with its label and number, also one whose value is not in the weights table, and shows "No pinned weights." without pins. It shows also without a candidate. Without an active model it shows "No score model is active: pinned weights have no effect.", and with one it does not.
- [x] A failed pin change shows an alert with "Could not change the pin." and the error, in the "New model" section for a row and in the "Pinned weights" section for the list. The pins stay as they were, and no preview is requested.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`score.ts` gains `ScorePin`, `ScorePreview.pins`, `savePandaScorePin` and `removePandaScorePin`; `PandaScorePage.tsx` holds the pin state, adds `PinControls` in a "Pin" column of the weights table and the "Pinned weights" section, and runs pin changes through the page's mutation with a background recalculation; covered in `PandaScorePage.test.tsx`.
