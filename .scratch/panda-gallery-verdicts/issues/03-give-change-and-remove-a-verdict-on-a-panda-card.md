# 03: Give, change and remove a verdict on a Panda card

What to build: each card in the Panda listing has two controls on its cover, "Not for me" and "Interesting". A click gives the verdict, a click on the other control changes it, and a click on the given one removes it. The card shows a verdict only after the collector stored it, becomes subdued when a verdict is given, and never moves. A failed save leaves the card as it was and shows a message that doesn't interrupt the user.

This is spec behaviours 1–15 and 17 (browser side), 19, 30, and 32–35. Behaviours 14 and 15 follow from one rule: a save never loads the listing again, in any view. The same rule gives behaviour 16 and the verdict-view parts of behaviours 1 and 5; ticket 05 pins those once the verdict views exist. Its parts are specified in spec **Design → Web** (the lines marked ticket 03) and **Fit** items 2–6, 8 and 10:

- `web/src/panda/api.ts`: `PandaVerdict`, `PandaGallery.verdict?`, and `savePandaVerdict(galleryID, verdict | null)` (`PUT` with `{"verdict": ...}` or `DELETE` on `/api/collector/catalog/verdicts/{id}`, `keepalive: true`, `collectorMessages`, no abort signal).
- `web/src/galleries/GalleryBox.tsx` and `Galleries.css`: the optional `actions` slot, rendered in a sibling of the link, and its styles (position over the cover, pointer events only on the controls, the hover and focus reveal with `opacity`, 44px on coarse pointers). Without `actions` the output is exactly today's. The `Tooltip` stays as it is.
- `web/src/panda/PandaCard.tsx`: the optional `onVerdict`, the two `IconButton`s with `aria-pressed` (icons, variants and colours as the spec lists them), and the card-local pending state with `aria-disabled` and a `Spinner`. No `disabled` and no `loading` prop. Without `onVerdict` the card renders no controls.
- `web/src/panda/PandaCatalog.tsx`: every listed card gets `onVerdict`; the hidden sizing card does not. On success, replace that gallery in the loaded result by gallery ID (the new `verdict`, and `displayed: true` after a set), and remove a failure message that names the same gallery. On failure, show the failure message of behaviour 33. A save that settles after a page change, or after another save settled, changes only its own gallery on the result that is current then. Never bump `attempt`, change the hash, or send a catalog or display request because of a save. Don't change the key handler: paging keys keep working with the focus on a control.
- `web/src/panda/PandaCatalog.css`: the failure message's wrapper, absolutely positioned directly above the `.gallery-pagination` row, as the spec describes it.

Don't use `useMutation` (one request at a time, aborted on unmount). Don't put the failure message in the page flow like the catalog's other alerts, and don't use a dialog: an in-flow alert changes the measured page capacity and loads the page again, and a dialog traps the focus while the user pages with the keys.

Test seam: a new `web/src/panda/PandaVerdicts.test.tsx` that renders `<App />` with its own stubbed `fetch`, built like `PandaCatalog.test.tsx` (copy its `useGalleryGrid` mock and its `FavoriteSyncAction` mock). The existing test files stay unedited. jsdom applies no stylesheet, so the reveal, the sizes, the position on the cover and the position of the failure message are covered by the spec's **Manual checks**, not by tests.

Blocked by: 02

Status: resolved

- [x] Each card in the "Panda galleries" list has two buttons named "Not for me" and "Interesting", in that order after the card's link. Neither is inside the link. For a gallery without a verdict both have `aria-pressed="false"`.
- [x] A catalog item with `verdict: "not_for_me"` renders "Not for me" with `aria-pressed="true"` and "Interesting" with `aria-pressed="false"`, in the view of all galleries and in the unseen view.
- [x] A click on "Interesting" on a gallery without a verdict sends one `PUT /api/collector/catalog/verdicts/<gallery id>` with the body `{"verdict":"interesting"}`, a JSON content type and `keepalive`. While the request is pending, both controls of that card have `aria-disabled="true"`, "Interesting" still has `aria-pressed="false"`, a second click on either control sends nothing, and the controls of another card still send their own request.
- [x] After the 204, "Interesting" has `aria-pressed="true"`, no control is `aria-disabled`, and the card's link has the class `gallery-box-subdued` and contains "Seen". When the saves of two cards were pending at the same time, both cards show their verdict after both settled.
- [x] With "Interesting" given, a click on "Not for me" sends a `PUT` with `{"verdict":"not_for_me"}`, and after the 204 only "Not for me" is pressed. A click on the pressed control sends `DELETE /api/collector/catalog/verdicts/<gallery id>`, and after the 204 neither control is pressed and the card is still subdued.
- [x] A save, successful or failed, sends no catalog request and no display request, and the list shows the same galleries in the same order afterwards. This holds at `#/panda?unseen=true` too: the card stays in its position with its verdict pressed.
- [x] When the save answers 502 `collector_unavailable`, the controls of the card are as before the click (same `aria-pressed`, not `aria-disabled`), and an element with role `alert` reads "Could not save the verdict for" with the gallery's title, followed by the collector message for that error. No element with role `dialog` or `alertdialog` appears. "Dismiss" removes the alert, and so does a later successful save for the same gallery.
- [x] With the focus on a verdict control, pressing `d` moves to the next page and sends the display record of the page. With the failure alert showing, `d` still moves to the next page, and the alert is still shown there.
- [x] `PandaCard` without `onVerdict` renders no button, and the cards of the local gallery listing contain no button (`PandaCard.test.tsx`, `Galleries.test.tsx` and `App.test.tsx` pass unedited).
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with every existing test file unedited.

## Resolution

`savePandaVerdict` and `PandaVerdict` in `web/src/panda/api.ts`; an optional `actions` slot on `GalleryBox` with its overlay and reveal styles; `PandaCard` verdict controls with card-local pending state; `PandaCatalog` patches the loaded result on a save and shows an out-of-flow failure alert above the pagination row. Tests in `web/src/panda/PandaVerdicts.test.tsx`.
