# 05: Favorites page syncs each category from its row and all categories with "Sync all"

What to build: on Collector → Favorites, the table always lists all ten categories, and the user syncs any single category with its row's "Sync" button. They request a full re-sync of one category from a small row menu beside "Download missing…", and sync all ten categories with one "Sync all" button. The Category dropdown, the Incremental/Full re-sync switch and the "Sync favorites" button are gone, and so is the "Show N empty" collapse. There is no longer a one-click full re-sync of all categories. The baseline, request and failure notices stay, reworded for the new controls.

Spec behaviours 2 and 18–24, and **Design → Web** (`RowActions.tsx`, `Favorites.tsx`):

- Remove the form card in `web/src/collector/Favorites.tsx`: the `Select`, the `SegmentedControl`, the "Sync favorites" button, and the `category` and `full` state. Drop the imports that become unused.
- In its place, add a `Card` with a solid "Sync all" button followed by the help text from behaviour 23. They wrap onto two lines at narrow widths.
- Remove the empty-category collapse: `emptyCategories`, `emptySynced`, `emptyCompleted`, `collapsible`, `showEmpty`, `categoryRanges`, the `saved` list that only fed them, and the collapsed "Categories … · Show N empty" row. Every category in `status.categories` gets its own row.
- Add `trailing?: ReactNode` to `web/src/collector/RowActions.tsx`. When it is set, render it in the delete position, in place of the delete `IconButton` or the empty `deleteSlot` spacer. Existing callers (`ReferenceImports`, `RawFeeds`, `MetadataProxies`) don't pass it and render unchanged.
- Each category row's actions, left to right:
  - `slot1Width={80}`, with a soft gray size-2 "Sync" button at full slot width that calls `sync(String(item.category), false)`.
  - `slot2Width={160}`, with the existing "Download missing…" button, unchanged.
  - `trailing`: a `DropdownMenu`. Its trigger is a soft gray size-2 `IconButton` holding `DotsHorizontalIcon` (`@radix-ui/react-icons`), with `aria-label={\`More actions for ${categoryName(item)}\`}`. `DropdownMenu.Content align="end"` holds one `DropdownMenu.Item`, "Full re-sync", which calls `sync(String(item.category), true)`.
  - The "Sync" button and the menu trigger are disabled whenever `disabled` is true, like "Download missing…".
- Widen the actions column from 190 to 320 (`columns={[undefined, 100, 190, 150, 140, 320]}`), and raise the `TableCard` `minWidth` from 900 to 1030.
- Route every sync control through the existing `syncMutation` and the `sync(category, full)` function. Notices name the category through `categoryName` (behaviour 22). The full re-sync notice suffix depends on the row's state at click time:
  - `state === 'queued' && !queued_full` gives " It replaces the queued sync.".
  - `(state === 'running' || state === 'waiting_cooldown') && !full` gives " It starts after the running sync.". The status reports a running sync with a future `retry_at` as `waiting_cooldown`, and `Enqueue` still queues the full re-sync as its follow-up.

  `sync` needs the row's `FavoriteCategory` (or an equivalent) for these. How it gets them is the implementer's choice. If `sync` calls `notifyFavoritesChanged()` (added by ticket 04), keep that call on success.
- Rename the status column header from "Sync" to "Status". Change the collecting footer text to end with "Use Sync all to resume failed categories."
- In `Favorites.test.tsx`, rewrite the tests for the removed controls and for the collapse:
  - "shows summary cards and collapses empty categories" becomes a test that every category in the status, empty ones included, gets its own row with a "Sync" button.
  - "submits an all-category sync and a selected full re-sync".
  - The "Sync favorites" assertions in "keeps favorites through a connection failure…".

  Leave `MissingFavoriteDownloads.test.tsx` passing unedited.

Blocked by: None

Status: resolved

- [x] The page has no "Favorite category" combobox, no "Incremental" or "Full re-sync" radio, and no "Sync favorites" button. The status column header reads "Status".
- [x] With three empty, idle, successfully synced categories (2, 3 and 4) beside categories 0 and 1, each of the five categories has its own row header and its own "Sync" button. There is no "Show 3 empty" button and no "Categories 2–4" row.
- [x] Clicking "Sync" in the "Manga" row (category 1) posts `/api/collector/favorites/1/sync` with `{"full":false}` and shows "Sync requested for Manga.".
- [x] Choosing "Full re-sync" from "More actions for Manga" posts `/api/collector/favorites/1/sync` with `{"full":true}` and shows "Full re-sync requested for Manga.".
- [x] The full re-sync notice ends with " It replaces the queued sync." when the row's state is `queued` with `queued_full` false. It ends with " It starts after the running sync." when the row's state is `running` with `full` false, and also when it is `waiting_cooldown` with `full` false. It has neither suffix when the row is idle.
- [x] "Sync all" posts `/api/collector/favorites/all/sync` with `{"full":false}` and shows "Sync requested for all ten categories.". No control on the page posts `/all/sync` with `{"full":true}`.
- [x] With the baseline `not_started` or `collecting`, a row "Sync", a "Full re-sync" and "Sync all" each show "Baseline sync requested for all ten categories.". The help text reads "Until the baseline is complete, any sync collects all ten categories without downloading favorites.".
- [x] While the collector is disconnected or the status is stale, "Sync all", every row "Sync" and every row menu trigger are disabled. They are enabled again after recovery.
- [x] When a sync request fails, its error shows as an alert.
- [x] Rows on the Reference imports, Raw feeds and Metadata proxies pages keep their actions, because their existing tests pass unedited.
- [x] `npm test`, `npm run lint` and `npm run build` pass, with `MissingFavoriteDownloads.test.tsx` unedited.

## Resolution

`Favorites.tsx` replaces the sync form and the empty-category collapse with a "Sync all" card and per-row "Sync" buttons and "Full re-sync" menus (`sync(full, item?)` builds the named notices); `RowActions.tsx` gains `trailing`; `Favorites.test.tsx` covers the new controls, notices, baseline, disabling and failure alert.
