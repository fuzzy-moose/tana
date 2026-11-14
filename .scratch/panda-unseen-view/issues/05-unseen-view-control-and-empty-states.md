# 05: Switch the Panda catalog to the unseen view, with its empty states

What to build: a view control in the Panda toolbar switches between all galleries and the unseen view. The listing link holds the choice. The unseen view combines with every filter, records pages through Next like every listing, offers no way back, and has its own empty states.

This is spec behaviours 1–3, 9 (the view control records nothing), 15, 16, 17 (browser side), 29–31 and 33. Its parts are specified in spec **Design → Web**:

- `web/src/navigation.ts`: `PandaListing` gains `unseen: boolean`. The route parses `params.get('unseen') === 'true'`. `pandaListingLink` appends `unseen=true` last, only when set.
- `web/src/panda/api.ts`: `listPandaCatalog` appends `unseen=true` last, only in the unseen view, so the existing URL assertions hold.
- `web/src/panda/PandaCatalog.tsx`: a Radix Themes `SegmentedControl` with "All" and "Unseen" in the toolbar. Changing it sets the hash to the listing with the new `unseen` and no cursor. Every existing link keeps `unseen` through the listing object. "Clear all filters" keeps the current view. In the unseen view, an empty result shows the states of behaviours 29–31 instead of the three states of the view of all galleries. Those three stay unchanged. Previous needs no special case: the collector sends no previous cursor in the unseen view (ticket 04).

Test seam: `PandaCatalog.test.tsx`. New tests install their own `fetchMock` implementation, and the existing tests stay unedited.

Blocked by: 03, 04

Status: resolved

- [x] At `#/panda`, the view control shows "All" selected and the catalog request has no `unseen` parameter. The navigation's Panda link is still `#/panda`.
- [x] Choosing "Unseen" on a page with a cursor moves to the listing hash with `unseen=true` as the last parameter and no `cursor`, and the next catalog request ends with `&unseen=true`. Choosing "All" removes `unseen` and the cursor. Neither choice sends a record request or a default-filter `PUT`.
- [x] In the unseen view, a search submit, a category change, "Bypass default" and "Apply default", "Include expunged", "Clear all filters", Next, and the stale-results "Newest galleries" link all lead to hashes that keep `unseen=true`.
- [x] After the unseen view was used, rendering the app at `#/panda` requests the catalog without `unseen`.
- [x] In the unseen view, Next on a response with a `next_cursor` sends the page's record and moves to that cursor. When the response at that cursor is empty with no cursors, the page shows "No more unseen Panda galleries." with a "Newest unseen galleries" link (the unseen listing without a cursor) and a "Show all galleries" link (the listing with `unseen` off, other fields kept, no cursor). Next and Previous are disabled, and no alert shows.
- [x] An empty unseen response without a cursor shows "No unseen Panda galleries." when there is no search, category or applied default filter, and "No unseen Panda galleries match." otherwise. Each offers "Show all galleries". An empty unseen response at a cursor with a search shows "No more unseen Panda galleries match.". None of these states shows "No Panda galleries yet.".
- [x] With the collector unreachable, a fresh visit to the unseen view shows the "Panda catalog unavailable." alert, as the view of all galleries does.
- [x] `npm test`, `npm run lint` and `npm run build` pass, with every existing test unedited.

## Resolution

`PandaListing` gained `unseen` (route parsing, `pandaListingLink`, `listPandaCatalog`, `App.tsx`); `PandaCatalog` has an All/Unseen `SegmentedControl`, keeps `unseen` in every listing link and "Clear all filters", and shows the unseen empty states of behaviours 29–31; covered by five new tests in `PandaCatalog.test.tsx`.
