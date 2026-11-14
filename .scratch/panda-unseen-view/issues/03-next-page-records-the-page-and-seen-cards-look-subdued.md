# 03: Going to the next page records the page, and displayed cards look subdued

What to build: in every Panda listing (the view of all galleries and each search), going to the next page marks the galleries that the page displayed as seen on the collector. The listing moves on at once, and a failed record shows a message. Cards of galleries already displayed have a subdued look.

This is spec behaviours 7–11, 27, 28 (normal look without `displayed`) and 32. Its parts are specified in spec **Design → Web**:

- `web/src/panda/api.ts`: `PandaGallery.displayed?: boolean`, and `recordPandaDisplays(galleryIDs: number[]): Promise<void>`. It posts `{"gallery_ids": [...]}` to `/api/collector/catalog/displays` with `keepalive: true` and `collectorMessages`, and takes no abort signal.
- `web/src/panda/PandaCatalog.tsx`: the next-page function from ticket 01 starts `recordPandaDisplays` with the IDs of `result.items` in page order, when there are any, and navigates without waiting.
- Record-failure alert, held as state in `PandaCatalog`: a `Callout` with `role="alert"` and no button, reading "Could not mark a page as seen. Its galleries stay unseen." followed by the error message. Place it directly after the stale-results `Callout`, above `.gallery-viewport`. Accept that its height changes the measured page size, so the current cursor reloads once when it appears and once when it goes. Each record, when it settles, sets the alert (rejected) or clears it (resolved), in the order the records settle. Don't track which record started last. The alert goes when `PandaCatalog` unmounts (the user leaves `#/panda`). `App.tsx` keeps it mounted across listing changes, and a record that settles after unmount needs no guard.
- `web/src/galleries/GalleryBox.tsx`: optional `subdued?: boolean`. When it is set, GalleryBox adds the class `gallery-box-subdued` to the anchor and renders `<VisuallyHidden>Seen</VisuallyHidden>` inside the anchor right after the title heading, so the card keeps its height. `web/src/panda/PandaCard.tsx` only passes `subdued={!!gallery.displayed}`, and `status` keeps its meaning. `Galleries.css` adds `.gallery-box-subdued .gallery-cover, .gallery-box-subdued h2 { opacity: 0.5; }`.

Test seams: `PandaCatalog.test.tsx`, where new tests install their own `fetchMock` implementation and the existing tests stay unedited (their catch-all answers the record with 200). `PandaCard.test.tsx` for the card.

Blocked by: 01, 02

Status: resolved

- [x] In the view of all galleries and with a search, a plain click on Next sends exactly one `POST /api/collector/catalog/displays` whose body is `{"gallery_ids":[...]}` with the displayed galleries' IDs in page order. The listing hash moves to the next cursor before that request resolves.
- [x] ArrowRight and `d` send the same single record and move to the next page.
- [x] Previous (the link, ArrowLeft and `a`), "Reload results", a search submit, a category change, toggling "Include expunged", and a click on a card send no record request.
- [x] A click on Next with Ctrl, Meta or Shift held, or with the middle button, sends no record request.
- [x] Next on a page that shows no galleries but has a next cursor moves to that cursor without a record request.
- [x] When the record request fails, the next page shows an alert containing "Could not mark a page as seen. Its galleries stay unseen." and the error message. After a later record succeeds, the alert is gone.
- [x] When the record of one page fails only after the record of the following page has succeeded, the alert shows.
- [x] After a failed record, leaving the Panda catalog for another Tana page and coming back shows no record-failure alert.
- [x] A `PandaCard` with `displayed: true` has the `gallery-box-subdued` class and "Seen" in its link's text. With `displayed` false or absent, it has neither.
- [x] `npm test`, `npm run lint` and `npm run build` pass, with every existing test unedited.

## Resolution

Added `recordPandaDisplays` and `PandaGallery.displayed` in `web/src/panda/api.ts`; the next-page function in `PandaCatalog.tsx` records the page's gallery IDs before navigating and shows a record-failure alert in settle order; `GalleryBox` takes `subdued` (class `gallery-box-subdued`, hidden "Seen"), which `PandaCard` sets from `displayed`, with the dimming in `Galleries.css`. Tests in `PandaCatalog.test.tsx` and `PandaCard.test.tsx`.
