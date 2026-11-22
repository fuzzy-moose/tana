# 05: The evaluation page pins only

What to build: the evaluation page (`#/panda/score`) has no override control: no "Overrides" section, no "Override" column, no override action in the add form. The dialog that turns the model off says what happens to the listing. Spec behaviours 1 to 5.

The override requests, types and rule names of `web/src/panda/score.ts` go, and so does `overrides` of `ScorePreview`. A test that used an override only to trigger a measurement uses a pin or the bonus; tests of the override controls go. The test fixture helpers' parameter `overrides` gets another name, for the grep below.

Blocked by: 03, 04

Status: resolved

- [x] The tabs are exactly "Weights", "Pins", "Favorite authors" and "Model", and the "Pins" tab holds the "Pinned weights" section.
- [x] The columns of the weights table are exactly "Tag, uploader or category", "Weight", "Favorites", "Catalog", "Active weight" and "Pin". Each row, also the row of a typed tag that no listed weight has, offers "Pin" and "Ignore", and "Remove pin" for a pinned value.
- [x] The form has the accessible name "Add pin" and exactly the actions "Pin" and "Ignore". "Pin" posts the typed weight and "Ignore" the weight 0 to `/api/collector/catalog/score/pins`. A failed save shows "Could not change the pin." with the server's message in the form.
- [x] The dialog "Turn off the score model?" reads "The model is deleted and can't be restored. The listing then hides nothing by score."
- [x] `grep -rni override web/src/panda` prints nothing.
- [x] `grep -rnE "ScoreRule|scoreRuleNames|(always|never)_hid|(Always|Never) hid" web/src --exclude-dir=node_modules` prints nothing.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`.

## Resolution

The evaluation page lost the "Overrides" section, the "Override" column and the override form actions: the form is "Add pin" with "Pin" and "Ignore", the tab is "Pins", and the turn-off dialog says the listing then hides nothing by score. `score.ts` lost the override requests, `ScoreOverride`, `ScoreRule`, `scoreRuleNames` and `ScorePreview.overrides`; tests that measured through an override now measure through a pin, and the override tests are gone.
