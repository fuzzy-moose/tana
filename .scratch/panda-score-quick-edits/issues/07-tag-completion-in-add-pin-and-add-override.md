# 07: Tag completion in "Add pin" and "Add override"

What to build: on the score page, the tag field of "Add pin" and of "Add override" suggests tags of the Panda catalog while the user types, as the search field of the Panda listing does, with the same keys. Choosing a suggestion puts the plain tag (`female:long hair`) in the field. While suggestions are open, Enter chooses one, and the next Enter submits the form. Without suggestions the field is a plain text field: a failed completion request shows nothing and does not interrupt typing. The uploader field gets no suggestions, and the category select stays.

This is spec behaviours 27 to 31, 34 and 35, specified in spec **Design → Web: the shared tag completion field** (`insert`) and **Design → Web: the score page → Forms**, and **Fit** 8.

- `web/src/galleries/TagCompletionField.tsx` gains the prop `insert?: 'term' | 'tag'`, default `'term'`. With `'term'` a chosen suggestion replaces the completed range with the suggestion's `term`, as since ticket 01. With `'tag'` it replaces the whole text of the field with `` `${namespace}:${value}` `` of the suggestion and puts the cursor at its end. The suggestion's `term` is search syntax (`female:"long hair$"`) and is not used.
- `web/src/panda/PandaScorePage.tsx`: `PinForm` and `OverrideForm` render `TagCompletionField` for the kind "Tag", with the `id` the field has today (`panda-score-pin-value`, `panda-score-override-value`), the form's text as `value` and `onChange`, `complete={completePandaSearch}` (from `./api`) and `insert="tag"`, under the same label "Tag (namespace:value)". For the kind "Uploader" they keep the plain text field, and for "Category" the select. Submit, the trimmed value, the disabled rule of the buttons and the messages do not change. The field's role is `combobox`, where it was `textbox`.
- The request is that of the listing search, with the field's text and cursor unchanged: `GET /api/collector/catalog/completions?q=<text>&cursor=<offset>`. Short forms, quoted values and values without a namespace are matched by the collector as there.

Test seams: `web/src/galleries/TagCompletionField.test.tsx` for `insert="tag"`, with a stubbed `complete`. `web/src/panda/PandaScorePage.test.tsx` for the forms, with a stubbed `fetch`. Two traps in that file:

- It answers `fetch` calls from a queue in call order (`respond`), so a completion request would take the answer meant for a save or a preview. Answer `/api/collector/catalog/completions` apart from the queue (an empty completion unless a test sets suggestions), and leave those calls out of `changeCalls()`.
- Existing tests find the tag field of the forms by the role `textbox`. They change to the role `combobox`; what they assert stays.

Blocked by: 01

Status: resolved

- [x] `TagCompletionField` with `insert="tag"`: choosing a suggestion with the namespace `female`, the value `long hair` and the term `female:"long hair$"` calls `onChange` with `female:long hair`, also when the text held several words, and the cursor is at the end of the text. With `insert` left out the field behaves as before: `GallerySearch.test.tsx` passes unedited.
- [x] Typing `f:long` in the tag field of "Add pin" requests `/api/collector/catalog/completions?q=f%3Along&cursor=6` and shows the suggestions that the request answers.
- [x] With suggestions open in the tag field of "Add pin" and a weight entered, Enter puts the marked suggestion's plain tag in the field and sends no pin. The next Enter posts `{"kind":"tag","value":"<the plain tag>","weight":<the weight>}` to `/api/collector/catalog/score/pins`.
- [x] In "Add override", clicking a suggestion puts its plain tag in the tag field, and "Add override" then posts it as the value to `/api/collector/catalog/score/overrides`.
- [x] With the kind "Uploader", the field of both forms is a text box without suggestions, and typing in it requests no completions. With the kind "Category" the select shows as before.
- [x] When the completion request fails, the form shows no alert, the typed text stays, and submitting posts the typed text.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`.

## Resolution

`TagCompletionField` gained `insert="tag"`, which puts the plain `namespace:value` of a chosen suggestion in the field; `PinForm` and `OverrideForm` render it for the kind "Tag" with `completePandaSearch`, and `PandaScorePage.test.tsx` answers completion requests apart from its queue.
