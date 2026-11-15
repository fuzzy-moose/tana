# 05: The "Pinned weights" list changes a pin in place

What to build: on the score page, each row of the "Pinned weights" list is one line with the label, a number field that holds the pinned weight, the weight that the new model calculates for the value, and "Remove". The user changes a pin by typing a number and pressing Enter. A save button shows while the field differs from the stored weight. Leaving the field saves nothing. A recalculation leaves alone what the user is typing in any pin field. The list has no "Ignore" button: the user types 0.

This is spec behaviours 11 to 18, and behaviours 1 to 5 for this list, specified in spec **Design → Web: the score page → "Pinned weights" list** and **Single-row layout**, and **Fit** 3, 6 and 7.

- `web/src/panda/score.ts`: `ScorePin` gains `model_weight?: number`. Ticket 03 puts it on the pins of a preview; the answer of a saved pin has none.
- `web/src/panda/PandaScorePage.tsx`, section "Pinned weights": the `<ul>` uses `.score-rows` and each label `.score-label` from ticket 04. A row is the editor of its pin, and every row has the same five cells in this order:
  - the label: `weightLabel(pin)`, with its full text as `title`;
  - a number field named "Weight of pin <label>" (the label as `weightLabel` gives it, `step="any"`, 6rem wide). It starts with the pinned weight and follows the pin by the rule of `PinControls`: the text is set again only when the pin's id or weight changes;
  - the place of the save control, an `IconButton` named "Save" (`CheckIcon`, title "Save the weight"). The button is rendered only while the field's text differs from the stored weight: a text that is the same number does not differ, and a text that is no number differs and disables the button (spec behaviour 14). The place keeps its width without the button, so the row does not shift when it shows;
  - the model weight cell: "Model " followed by the model weight, formatted like the weights of the table (`formatWeight`), with the title "Weight in the new model". Without a model weight the cell stays in the row and is empty, so that "Remove" stands in one column;
  - the button "Remove", as today.
- The model weight of a row comes from the last preview, not from the pin state: `PinnedWeights` also gets `preview.pins`, and a row shows the `model_weight` of the preview's pin with its kind and label, matched as `findPin` matches. The model weights do not depend on the pins, so a saved pin keeps its model weight, and a pin that is new in this visit has none until the calculation that follows answers. `savePin`, its updater and the pin state do not change.
- Saving: Enter in the field and the save control save through the page's pin change, as action `pin-list`, with the pin's kind, its label as the value and the field's number, only when the text is a number that differs from the stored weight and no change is being saved. Blur does nothing.
- Messages stay where they are: a failed change of the list shows "Could not change the pin." with the error below the list.

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`. The list no longer shows a pin as one text ("female:glasses +2.00"): the helper `listedPins()` and the assertions that read a row by that text change to read the label and the field. What those tests assert stays: which pins the list holds, in which order, with which weights. Layout is a manual check (spec **Manual checks**, "One line per entry", "Columns", "Editing pins against the real collector").

Blocked by: 03, 04

Status: resolved

- [x] Each row of the list shows the pin's label with its full text as `title`, a field "Weight of pin <label>" that holds the stored weight, and "Remove", for a tag pin, an uploader pin and a category pin. The list has no "Ignore" button.
- [x] A row whose pin has `model_weight` in the preview shows "Model " with that weight (sign and two decimals, for example "Model +0.50"). A row whose pin has none shows no "Model" text.
- [x] A pin that is new in this visit, saved from a row of the weights table, shows its model weight in its row of the list once the preview that follows lists the pin with `model_weight`.
- [x] Typing another number in a row's field shows the button "Save". Enter posts the pin's kind, its label as the value and the number to `/api/collector/catalog/score/pins`, and so does a click on "Save". After the answer the field holds the stored weight, "Save" is gone, the row shows its model weight also before the calculation that follows answers, and the page requests the preview again.
- [x] Typing 0 and saving posts the weight 0.
- [x] Leaving a changed field sends nothing: the text and "Save" stay.
- [x] A text that is the same number as the stored weight ("2.0" for 2) shows no "Save", and Enter sends nothing. An empty field shows "Save" disabled, and Enter sends nothing.
- [x] Text typed in a pin field stays when a recalculation answers with that pin unchanged, in the "Pinned weights" list and in a row of the weights table. A preview in which that pin has another weight replaces the text with that weight.
- [x] A failed save shows an alert with "Could not change the pin." and the error in the "Pinned weights" section, keeps the typed text and "Save", and requests no preview.
- [x] The list keeps the order of the preview, and a pin saved from elsewhere on the page is added last, as before.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`.

## Resolution

Each row of the "Pinned weights" list is a `PinnedWeight` editor (label, "Weight of pin <label>" field, "Save" icon slot, "Model <weight>" from `preview.pins`, "Remove") in `web/src/panda/PandaScorePage.tsx`; `ScorePin` gained `model_weight`; `PandaScorePage.test.tsx` reads list rows by label and field and covers saving, no-save cases, recalculation and failures.
