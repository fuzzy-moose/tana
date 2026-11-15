# 14: The local gallery detail view shows tag weights and pins tags

What to build: when the collector answers, the tags section of a local gallery's detail view shows beside each tag its current weight in the Panda score, and marks the pinned tags. The weight is a button that opens a small editor: a number field that starts at the current weight, "Pin", "Ignore" (a pin of 0) and, for a pinned tag, "Remove pin". After a change the section shows the new weights. A tag that the Panda catalog does not have shows that reason and gets no editor. Without an active model the section says that pins have no effect. The tag selection and "Search selected tags" work as today, and when the collector is not configured, not reachable or fails, the view looks exactly as today. Only tags get a pin here.

This is spec behaviours 41 to 50. Its parts are specified in spec **Design → Web**, **Detail view (ticket 14)**:

- `web/src/panda/score.ts`: `ScoreTagWeight`, `ScoreTagWeights` and `getPandaTagWeights(tags, signal)` (`POST /api/collector/catalog/score/tags` with `{"tags": [...]}`), with `collectorMessages`. The pin functions are those of ticket 11.
- `web/src/galleries/GalleryDetail.tsx` (and `GalleryDetail.css` as needed):
  - After the gallery is loaded, and only when it has tags, the view requests the weights of all its tags, each as `namespace:value`, in one request that does not delay the page.
  - The view takes an answer only when it is a valid tag weights answer: `typeof answer.active === 'boolean'` and `Array.isArray(answer.tags)`. A failed request and every other answer leave the view as it is today: no weight, no button, no notice and no message. The view uses an entry only for the tag whose `namespace:value` equals the entry's `tag`. A tag without such an entry looks as today.
  - With a valid answer and `active === false`, the tags section shows "No score model is active: pinned weights have no effect."
  - A tag with `known: true` gets a button beside its toggle, in the same list item and outside the toggle's link. Its text is the weight with sign and two decimals ("+0.42", "-1.20", "+0.00"), or "–" for null. Its accessible name is "Weight of <namespace:value>: <text>", and for a pinned tag "Pinned weight of <namespace:value>: <text>", with `DrawingPinFilledIcon` before the text.
  - The button opens a Radix `Popover` with the pin editor: a number field "Weight" (`step="any"`) and the buttons "Pin" (disabled while the field holds no number), "Ignore" and, for a pinned tag, "Remove pin". The field starts at the pin's number as stored for a pinned tag, at the weight rounded to two decimals for another tag with a weight, and at 0 for a tag without one. "Pin" and "Ignore" call `savePandaScorePin` with the kind `tag`, the tag's `namespace:value` and the number, and "Remove pin" calls `removePandaScorePin` with the pin's ID.
  - After a successful change the editor closes and the view requests the tag weights again and shows the answer. A failed change shows "Could not change the pin." with the error in an alert inside the editor, which stays open, and the weights stay as they were.
  - A tag with `known: false` gets no button. Its list item carries the visually hidden text " (not in the Panda catalog)" and an icon with the title "Not in the Panda catalog" (`@radix-ui/react-icons`, the implementer's choice).
  - The toggle link, `toggleTag`, "Search selected tags", "Clear" and `GalleryPandaLink` do not change. The view gets no control for the uploader or the category.

Existing tests: one assertion changes. The first test of `web/src/galleries/GalleryDetail.test.tsx` expects one `fetch` call for a gallery with tags and without a Panda candidate. The tag weights request is a second call, so the assertion becomes that no request went to `/api/collector/catalog/lookup`. Every other existing test of `GalleryDetail.test.tsx` and `App.test.tsx` keeps its assertions: their stubs answer the tag weights request with JSON that is no valid tag weights answer, so they see today's view. If a stub must answer the tag weights request differently for a test to pass, it answers that request by path.

Test seam: new cases in `web/src/galleries/GalleryDetail.test.tsx`, which renders `<GalleryDetail id={1} />` with a stubbed `fetch` that answers by path. The layout of the buttons beside the tags and the position of the popover are manual checks (spec **Manual checks**).

Blocked by: 11

Status: resolved

- [x] For a gallery with tags, the view posts all its tags as `namespace:value` in one request to `/api/collector/catalog/score/tags`. The gallery's heading and its tags show before that request is answered. A gallery without tags sends no such request.
- [x] With a valid answer, a tag that the Panda catalog has shows a button with its weight: "Weight of <namespace:value>: <weight>" with sign and two decimals, or "–" for a tag without a weight. A pinned tag's button is named "Pinned weight of <namespace:value>: <weight>" and has an icon. A tag that the Panda catalog does not have has no weight button and carries the text "not in the Panda catalog". The view has no pin control outside the tags section.
- [x] With `active: false` the section shows "No score model is active: pinned weights have no effect.", and with `active: true` it does not.
- [x] The editor's field "Weight" starts at the pin's number for a pinned tag, at the weight rounded to two decimals for another tag with a weight, and at 0 for a tag without one. Only the editor of a pinned tag has "Remove pin".
- [x] "Pin" posts the kind `tag`, the tag's `namespace:value` and the field's number to `/api/collector/catalog/score/pins`. "Ignore" posts the weight 0. "Remove pin" sends `DELETE /api/collector/catalog/score/pins/{id}` with the pin's ID. After each success the editor closes, the view requests the tag weights again, and it shows the new answer's weights and marks.
- [x] A failed change shows an alert in the open editor with "Could not change the pin." and the error, "No gallery in the Panda catalog has this tag." for 404 `tag_not_found`. The view requests no new tag weights, and the tag's button keeps its text.
- [x] With the weights shown, a click on a tag's toggle selects it and "Search selected tags" has today's link. A click on a tag's weight button does not change the selection.
- [x] A failed tag weights request (503 `collector_not_configured`, 502 `collector_unavailable`) and a 200 answer that is no valid tag weights answer (no boolean `active`, or no `tags` array) leave the tags section as today: each list item's whole text is the tag's value, and there is no weight button, no notice and no alert.
- [x] The existing tests of `GalleryDetail.test.tsx` and `App.test.tsx` pass with their assertions unchanged, except the one assertion named above.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`.

## Resolution

`getPandaTagWeights` and its types in `web/src/panda/score.ts`; `GalleryDetail.tsx` requests the tag weights after the gallery loads and shows a weight button with a popover pin editor (`TagWeight`) beside each known tag, an unknown-tag mark, and the no-active-model notice; new cases in `GalleryDetail.test.tsx`, whose first test now checks that no lookup request was sent.
