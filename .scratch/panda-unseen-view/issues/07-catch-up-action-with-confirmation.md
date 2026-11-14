# 07: Catch up from the unseen view after a confirmation

What to build: the unseen view gets a "Catch up…" button. After a confirmation that states its effect, it makes the current page and every older Panda gallery seen, and then shows the newest unseen galleries.

This is spec behaviours 19–21 (browser side), 25, 26, and 9 (a catch-up sends no display record). Its parts are specified in spec **Design → Web**:

- `web/src/panda/api.ts`: `catchUpPanda(galleryID: number, signal: AbortSignal): Promise<void>`. It posts `{"gallery_id": n}` to `/api/collector/catalog/catch-up` with `collectorMessages` plus `gallery_not_found: 'This gallery is no longer in the Panda catalog. Reload and try again.'`.
- `web/src/panda/PandaCatalog.tsx`:
  - In the unseen view only, the toolbar has a "Catch up…" button. It is disabled while loading, while the error alert shows, and when the page shows no galleries.
  - The button opens an `AlertDialog`, built like the delete confirmation in `collector/Downloads.tsx`, titled "Catch up?", with the text of spec behaviour 20 and the buttons "Cancel" and "Catch up". The date is the first gallery's `posted_at`, or its `first_seen_at`, formatted with `toLocaleString()`.
  - "Catch up" runs `catchUpPanda(result.items[0].gallery_id)` through `useMutation` (`../collector/useMutation`). While it is `pending`, "Cancel" and "Catch up" are disabled and the dialog ignores requests to close (Escape). The dialog's "Catch up" is also disabled whenever the toolbar's "Catch up…" would be.
  - On success it closes the dialog and sets the hash to the listing without a cursor. When the hash is already cursor-less, it bumps `attempt` so the listing reloads.
  - On failure the dialog stays open and shows, inside the dialog, text with `role="alert"`: "Catch-up failed. Nothing was marked as seen." followed by the error. The page behind it doesn't change. The alert goes when the next try starts, and the dialog always opens without an alert from an earlier try. The alert is inside the dialog, so it never changes the page size.
  - Add `[role="alertdialog"]` to the key handler's ignore selector.

Test seam: `PandaCatalog.test.tsx`. New tests install their own `fetchMock` implementation, and the existing tests stay unedited.

Blocked by: 05, 06

Status: resolved

- [x] In the unseen view with galleries, "Catch up…" opens a dialog whose text contains the newest gallery's date as `toLocaleString()` renders it, and "This can't be undone.". The view of all galleries renders no "Catch up…" button.
- [x] "Catch up…" is disabled when the unseen page shows no galleries, and when the page shows the stale-results alert after a failed load.
- [x] "Cancel" closes the dialog and sends no request.
- [x] "Catch up" sends exactly one `POST /api/collector/catalog/catch-up` with `{"gallery_id":<first gallery on the page>}`. The next catalog request is the unseen listing with the same search and filters and no cursor, both when the page had a cursor and when it didn't.
- [x] Opening, cancelling and confirming the catch-up send no display record.
- [x] While the catch-up request is pending, "Cancel" and "Catch up" are disabled and Escape doesn't close the dialog.
- [x] When the catch-up request fails, the dialog stays open and shows an alert containing "Catch-up failed. Nothing was marked as seen." and the error message. The page keeps its galleries and cursor, and no catalog request follows. For a 404 `gallery_not_found` response, the error message is "This gallery is no longer in the Panda catalog. Reload and try again.".
- [x] After a failure, a second "Catch up" removes the alert while it is pending, and a success then closes the dialog and loads the newest unseen galleries. After a failure, "Cancel" and reopening the dialog shows no alert.
- [x] ArrowRight, ArrowLeft, `a` and `d` pressed while the dialog is open neither change the hash nor send a record.
- [x] `npm test`, `npm run lint` and `npm run build` pass, with every existing test unedited.

## Resolution

Added `catchUpPanda` to `web/src/panda/api.ts` and the unseen-view "Catch up…" button with its `AlertDialog` confirmation (pending lock, in-dialog failure alert, reload of the newest unseen galleries) to `PandaCatalog.tsx`, covered by new tests in `PandaCatalog.test.tsx`.
