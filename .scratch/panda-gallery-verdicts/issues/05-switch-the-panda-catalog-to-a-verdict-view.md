# 05: Switch the Panda catalog to a verdict view, with its empty states

What to build: the view control in the Panda toolbar gains "Interesting" and "Not for me". Each lists only the galleries with that verdict. The listing link holds the view, the view combines with the search and every filter, pages like the view of all galleries, shows its cards in the normal look with their verdict controls, and has its own empty states. A card whose verdict the user changes or removes there stays in its position.

This is spec behaviours 16, 20–22 and 24–29, and the verdict-view parts of behaviours 1 and 5. Its parts are specified in spec **Design → Web** (the lines marked ticket 05):

- `web/src/navigation.ts`: `PandaView` gains `'interesting' | 'not_for_me'`. The route parses a `verdict` parameter with one of these values as the view, before it looks at `unseen`; any other value is ignored. `pandaListingLink` writes `verdict=<view>` as the last parameter for a verdict view, and never together with `unseen`.
- `web/src/panda/api.ts`: `listPandaCatalog` appends `verdict=<view>` last, only in a verdict view, so the existing URL assertions hold.
- `web/src/panda/PandaCard.tsx`: an optional `subdued?: boolean` that defaults to `!!gallery.displayed`.
- `web/src/panda/PandaCatalog.tsx`: the view control has four items, "All", "Unseen", "Interesting", "Not for me", and sets the hash to the listing with the chosen view and no cursor. Every existing link keeps the view through the listing object, and "Clear all filters" keeps it. Cards get `subdued={false}` in a verdict view. In a verdict view without a cursor, an empty result shows the states of behaviours 27 and 28; with a cursor it shows the existing states of the view of all galleries. The unseen states and the three states of the view of all galleries stay unchanged. "Catch up…" stays tied to the unseen view. Let the heading and the view control wrap at phone width, so the toolbar doesn't overflow.

The verdict controls and the save handler of ticket 03 need no change: a save patches the loaded page in every view and never loads the listing again, which is what keeps a card in a verdict view after its verdict is changed or removed. The catalog's error display doesn't depend on the view either (behaviour 35), so it needs no code and no test here.

Empty-state texts:

- Without search, categories and applied default filter: heading "No Panda gallery has this verdict.", text "Mark a gallery as Interesting on its card, and it appears here." (in the other view "Mark a gallery as Not for me on its card, and it appears here."), and a "Show all galleries" link to the view of all galleries with no cursor.
- With one of them: heading "No Panda gallery with this verdict matches.", text "Try different titles, tags, or categories, or clear all filters.", and a "Clear all filters" link to the same verdict view without search, categories and "Include expunged", with the default filter bypassed (today's `clearHref`).

Test seam: `web/src/panda/PandaVerdicts.test.tsx`, which ticket 03 created. Add to it and reuse its stubbed `fetch` and its mocks. Leave every existing test unedited.

Blocked by: 01, 03, 04

Status: resolved

- [x] The view control has the four options "All", "Unseen", "Interesting" and "Not for me", in this order. At `#/panda` "All" is selected and the catalog request has no `verdict` parameter.
- [x] Choosing "Interesting" on a page with a cursor moves to the listing hash with `verdict=interesting` as the last parameter and no `cursor`, and the next catalog request ends with `&verdict=interesting`. "Not for me" gives `verdict=not_for_me`. Neither choice sends a display record or a default-filter `PUT`.
- [x] Choosing "Unseen" in a verdict view moves to a hash with `unseen=true` and no `verdict`. Choosing "All" removes both. Choosing a verdict view in the unseen view removes `unseen`.
- [x] Rendering the app at `#/panda?q=s&category=manga&verdict=not_for_me` selects "Not for me" and requests the catalog with that search, category and verdict. A hash with `verdict=interesting&unseen=true` selects "Interesting" and sends no `unseen` parameter. A hash with `verdict=maybe` selects "All" and sends no `verdict` parameter.
- [x] In a verdict view, a search submit leads to a hash that keeps the `verdict` parameter, and the toolbar's "Clear all filters" link keeps it while it drops the search.
- [x] In a verdict view, Next on a response with a `next_cursor` sends the page's display record and moves to that cursor in the same verdict view, Previous is a link when the response has a `previous_cursor`, and there is no "Catch up…" button.
- [x] In a verdict view, each card has the buttons "Not for me" and "Interesting", and the one of the view's verdict has `aria-pressed="true"`. A click that removes the verdict, and a click that changes it to the other one, each leave the list with the same galleries in the same order, show the new pressed state on the card, and send no catalog request.
- [x] In a verdict view, a card of an item with `displayed: true` has no `gallery-box-subdued` class and no "Seen" text. The same item in the view of all galleries has both.
- [x] An empty verdict response without a cursor shows "No Panda gallery has this verdict." with the text for the current verdict and a "Show all galleries" link, when there is no search, category or applied default filter. Otherwise it shows "No Panda gallery with this verdict matches." with a "Clear all filters" link whose hash keeps the `verdict` parameter. Neither state shows "No Panda galleries yet." or an unseen-view text.
- [x] An empty verdict response at a cursor shows "No more matching Panda galleries." with a "Newest galleries" link that keeps the `verdict` parameter and has no cursor.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with every existing test unedited.

## Resolution

`PandaView` gains the two verdict views (`navigation.ts`, `panda/api.ts`), `PandaCard` takes `subdued`, and `PandaCatalog` offers the four-item view control, the verdict empty states and a wrapping heading; covered in `PandaVerdicts.test.tsx`.
