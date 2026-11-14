# 13: Score overrides on the evaluation page

What to build: on the evaluation page the user makes the listing always hide or never hide the galleries of one tag, uploader or category: with two buttons on each row of the weights table, and with a form for a value that is not in the table. The page lists the current overrides and removes one on request. Each change shows in the page's numbers after the new calculation, and a failed change leaves the overrides as they were and shows a message.

This is spec behaviours 24 to 26 (the web side), 28 and 48, and behaviour 17 for an override change. Its parts are specified in spec **Design → Web**:

- `web/src/panda/score.ts`: `savePandaScoreOverride({ kind, value, rule }, signal)` (`POST /api/collector/catalog/score/overrides`) and `removePandaScoreOverride(id, signal)` (`DELETE /api/collector/catalog/score/overrides/{id}`), with `collectorMessages` plus: `tag_not_found` → "No gallery in the Panda catalog has this tag.", `invalid_override` → "Enter a tag as namespace:value, an uploader name, or a Panda gallery category."
- `web/src/panda/PandaScorePage.tsx`:
  - Each row of the weights table gets two small toggle buttons, "Always hide" and "Never hide", with `aria-pressed`. A button is pressed when `overrides` of the preview holds an override with the row's `kind` and `label` and that rule. A click on a button that is not pressed saves the override with the row's kind, its label as the value, and the button's rule. A click on the pressed button removes that override by its `id`.
  - Section "Overrides", shown also without a candidate and without an active model: the list of `overrides`, each as "Always hide" or "Never hide", its tag, "Uploader <label>" or "Category <label>", and a button "Remove". An empty list reads "No overrides."
  - Below the list, a form "Add override": a choice of kind (Tag, Uploader, Category), a text field for a tag ("namespace:value") or an uploader ("exactly as Panda writes it"), a choice among the entries of `pandaCategories` (`web/src/panda/categories.ts`) for a category, a choice of rule (Always hide, Never hide), and a button "Add override". An empty value disables the button. Use Radix Themes components that the existing tests already drive (`SegmentedControl`, `TextField`, `Button`, `DropdownMenu`) or native `select` elements; the choice is the implementer's.
  - After each successful change the page requests the preview again (with the switch's current value), which brings the new list, the pressed states and the new shares. This preview is a new calculation, not the start of a visit: the threshold fields of ticket 12 keep what the user entered. The form's value is cleared after a successful add.
  - A failed change shows "Could not change the override." with the error in an alert near the control that was used, requests no preview, and leaves the list and the pressed states as they were. While a change runs, the override controls ignore clicks. The override changes run through the page's one `useMutation` of ticket 12, so no two changes of the page run at once.

Test seam: `web/src/panda/PandaScorePage.test.tsx` (the file of ticket 11) with a stubbed `fetch`. Existing assertions stay unchanged; new cases may be added to existing test files. Layout of the row buttons on a narrow window is a manual check.

Blocked by: 12

Status: resolved

- [x] The "Overrides" section lists each override of the preview with its rule and its value, uploaders and categories with their prefix, and "No overrides." for an empty list. It shows with `no_model` `no_verdicts` too.
- [x] "Always hide" on the row of the tag `female:glasses` posts `{"kind":"tag","value":"female:glasses","rule":"always_hide"}` to `/api/collector/catalog/score/overrides`, and after the 204 the page posts the preview again. With the override in the new preview, the row's "Always hide" button is pressed and the list shows it.
- [x] After the user entered a threshold for the new model, an override change and its new preview (another candidate `id`, another suggested threshold) leave the "Threshold" field at the entered value, and the shares follow the new preview at that value.
- [x] A click on a pressed row button sends `DELETE /api/collector/catalog/score/overrides/<id>` with the override's ID, then the preview again.
- [x] "Never hide" on a row whose "Always hide" is pressed posts the override with the rule `never_hide` and sends no `DELETE`.
- [x] "Remove" in the list sends the `DELETE` for that override, then the preview again.
- [x] The form posts the chosen kind, the trimmed value and the chosen rule. For the kind Category the value comes from the category choice. With an empty value the button is disabled. After a 204 the text field is empty.
- [x] A 404 `tag_not_found` from the form shows "Could not change the override." with "No gallery in the Panda catalog has this tag.", the list is unchanged, no preview is requested, and the text field keeps its value. A 502 on a row button shows the message with the collector error and leaves the button not pressed.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`web/src/panda/score.ts` gains `savePandaScoreOverride` and `removePandaScoreOverride` with the two new messages. `PandaScorePage.tsx` adds the "Always hide"/"Never hide" toggles to each weights row, an "Overrides" section with the list, "Remove" and the "Add override" form (SegmentedControl for kind and rule, native select for the category); override changes run through the page's one mutation, calculate again with the switch's value, and leave the threshold fields as entered. Cases added to `PandaScorePage.test.tsx`; its `rows()` helper reads the first six cells, so the existing assertions are unchanged.
