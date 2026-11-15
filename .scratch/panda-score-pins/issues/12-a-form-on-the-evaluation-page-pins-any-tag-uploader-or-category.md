# 12: A form on the evaluation page pins any tag, uploader or category

What to build: in the "Pinned weights" section of the evaluation page, a form "Add pin" pins a tag, uploader or category that is not in the weights table: the user chooses the kind, enters the value and a number, and adds the pin. The form has the value rules and the messages of the override form, also for a tag that the Panda catalog does not have. A pin from the form shows at once in the list, and the page calculates again in the background, as for the row controls of ticket 11.

This is spec behaviour 33, and behaviours 10 and 40 for the form. Its parts are specified in spec **Design → Web**, item *Form "Add pin"* of **Evaluation page**:

- `web/src/panda/PandaScorePage.tsx`: a form with the accessible name "Add pin" and the heading "Add pin", in the "Pinned weights" section below the list, shown also without a candidate and without an active model:
  - a choice of kind (Tag, Uploader, Category), as in the override form;
  - for a tag or an uploader a text field with the override form's label ("Tag (namespace:value)" or "Uploader (exactly as Panda writes it)"), and for a category a choice among the entries of `pandaCategories` labelled "Category". Its element IDs differ from the override form's;
  - a number field "Weight" with `step="any"`;
  - a button "Add pin", disabled while the value is empty or the weight field holds no number.
- The form saves with `savePandaScorePin` (kind, the trimmed value or the chosen category, the number) through the pin change of ticket 11: the answer goes into the pin state at once, the page calculates again in the background, and the threshold fields keep their values. After a successful add, the text field and the weight field are empty.
- A failed add shows "Could not change the pin." with the error in an alert inside the form, keeps the entered value and weight, changes no state and requests no preview. The texts for `tag_not_found` and `invalid_pin` are those of ticket 11, which are the override form's texts.

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`, with cases like those of the override form ("the form adds an override for a tag, an uploader or a category", "a failed override change shows a message, keeps the state and calculates nothing"). Existing assertions stay unchanged: the override form keeps its accessible name "Add override" and its one text field.

Blocked by: 11

Status: resolved

- [x] The form posts the chosen kind, the trimmed value and the number to `/api/collector/catalog/score/pins`: for a tag, for an uploader, and for a category, whose value comes from the category choice. It shows also without a candidate and without an active model.
- [x] "Add pin" is disabled while the value is empty or the weight field holds no number. A weight of 0 is a number and is posted.
- [x] After a successful add, the "Pinned weights" list shows the new pin before the calculation that follows has answered, and the text field and the weight field are empty.
- [x] A failed add shows an alert inside the form: "Could not change the pin. No gallery in the Panda catalog has this tag." for 404 `tag_not_found`, and "Could not change the pin. Enter a tag as namespace:value, an uploader name, or a Panda gallery category." for 400 `invalid_pin`. The list stays as it was, no preview is requested, and the fields keep their values.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`PandaScorePage.tsx` gains a `PinForm` ("Add pin") in the "Pinned weights" section that saves through `runPinChange` with the new change `pin-form` and clears its value and weight fields on success; `PandaScorePage.test.tsx` covers adding a tag, uploader and category pin and the failure messages.
