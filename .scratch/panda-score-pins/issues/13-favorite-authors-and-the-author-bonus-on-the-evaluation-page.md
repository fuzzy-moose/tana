# 13: Favorite authors and the author bonus on the evaluation page

What to build: a section "Favorite authors" on the evaluation page shows how many favorite authors there are and lists them, those with the most Panda favorites first, each with its favorite count, its effective weight and the pin controls, so the user pins an author there. In the same section the user sets the author bonus with a number field and "Save bonus", and turns it off with "Turn off bonus". The section says whether the bonus is on, and that it has no effect while no model is active. After a bonus change the page calculates again, so its numbers count the new bonus.

This is spec behaviours 36 to 38, and the bonus side of behaviours 39 and 40. Its parts are specified in spec **Design → Web**, item *Section "Favorite authors"* of **Evaluation page**:

- `web/src/panda/score.ts`: `ScoreFavoriteAuthor`, the fields `author_bonus`, `favorite_author_count` and `favorite_authors` of `ScorePreview`, and `savePandaAuthorBonus(bonus, signal)` (`PUT /api/collector/catalog/score/author-bonus` with `{"bonus": number or null}`), with `collectorMessages` plus `invalid_author_bonus` → "Enter a number above 0."
- `web/src/panda/PandaScorePage.tsx`: the section "Favorite authors" after "Pinned weights", shown also without a candidate and without an active model:
  - the text "<count> favorite authors: artist tags that at least 2 Panda favorites carry.", with `favorite_author_count` in the page's number format;
  - without an active model, "No score model is active: the author bonus has no effect.";
  - a number field "Author bonus" (step 0.1). It holds the saved bonus and is empty while the bonus is off, and it starts again from the preview after a successful bonus change. A button "Save bonus", disabled unless the field holds a number above 0. A button "Turn off bonus" while the bonus is on. Beside them "The author bonus is off." or "Each favorite author's tag weighs at least <formatWeight(bonus)>.";
  - the list of `favorite_authors`, as a list and not a table: each item shows the label, "<n> favorites", the weight (`formatWeight`, or "–" for null), and the pin controls of ticket 11 for the kind `tag` and the label. It shows 50 authors, and "Show more" adds 50.
- A bonus change runs like an override change: through `runChange`, with its controls disabled during a calculation and during a change, followed by a new calculation that keeps the threshold fields. A failed change shows "Could not save the author bonus." with the error in the section, changes nothing and requests no preview.
- A pin from the list is a pin change of ticket 11: it shows at once in the "Pinned weights" list and in the author's pin field, and the page calculates again in the background. A failed pin change from the list shows "Could not change the pin." with the error in the "Favorite authors" section.

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`. Its fixtures `preview()` and `measured()` gain `author_bonus: null`, `favorite_author_count: 0` and `favorite_authors: []`. Existing assertions stay unchanged: the button of the active model keeps the exact name "Turn off", and `screen.getByRole('table')` needs the page to have one table.

Blocked by: 11

Status: resolved

- [x] The section shows the number of favorite authors in the text "<count> favorite authors: artist tags that at least 2 Panda favorites carry." and lists the authors of the preview in the preview's order, each with its label, "<n> favorites" and its weight, or "–" for an author without one. It shows also without a candidate.
- [x] The list shows 50 authors, and "Show more" adds 50.
- [x] While the bonus is off, the field "Author bonus" is empty, the section shows "The author bonus is off.", and there is no "Turn off bonus". "Save bonus" is disabled unless the field holds a number above 0.
- [x] "Save bonus" sends `PUT /api/collector/catalog/score/author-bonus` with the field's number, and "Turn off bonus" sends it with `{"bonus":null}`. After each success the page requests the preview again, and a threshold that the user entered stays in its field. With the bonus on in the new preview, the section shows "Each favorite author's tag weighs at least <bonus>." and "Turn off bonus".
- [x] A failed bonus change shows an alert in the section with "Could not save the author bonus." and the error, "Enter a number above 0." for 400 `invalid_author_bonus`. No preview is requested.
- [x] The pin controls of an author pin its tag: "Pin" posts the kind `tag`, the author's label and the field's number to `/api/collector/catalog/score/pins`, and the "Pinned weights" list shows the pin before the calculation that follows has answered.
- [x] Without an active model the section shows "No score model is active: the author bonus has no effect.", and with one it does not.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`web/src/panda/score.ts` gains `ScoreFavoriteAuthor`, the bonus and favorite author fields of `ScorePreview` and `savePandaAuthorBonus`; `PandaScorePage.tsx` gains the section "Favorite authors" with the bonus form (a `bonus` change that keeps the threshold fields and restarts the bonus field from the next preview) and the author list with pin controls (`pin-author`); tests in `PandaScorePage.test.tsx`.
