# One-click sync of Panda automatic download categories

## Problem

To sync the Panda favorite categories that download automatically, the user has to open Collector → Favorites, pick a category or "All ten categories" in a dropdown, choose Incremental or Full re-sync, and submit. Several Panda automatic download categories need one round per category, or a sync of all ten. Outside the Collector, Tana shows nothing about favorite syncs, so the user has to open Collector → Favorites to see whether a sync is queued, running, waiting for the authenticated cooldown, or failed.

## Solution

A quick action in the main sidenav, below Galleries, Panda, Libraries and Collector, starts an incremental Panda favorite sync of every saved Panda automatic download category with one click. It shows their live sync state from every page. On the Favorites page, each category row has its own "Sync" button and a small menu with "Full re-sync". A page-level "Sync all" button replaces the dropdown and the mode switch.

## Fit

Fits, with one small prefactor. Some parts of the design exist only because of how the code is built today:

1. **The sync endpoint takes one category or `all`.** Syncing several Panda automatic download categories this way would take one request per category, and one request could succeed while another fails. *Not a workaround:* the feature adds one route that syncs the saved download categories. The collector reads them and admits them in a single transaction, through the favorites service's existing multi-category `enqueue` path.
2. **The helpers the quick action needs sit inside the lazy Collector chunk's modules.** `categoryName` lives in `Favorites.tsx`, and a copy sits in `MissingFavoriteDownloads.tsx`. `useCollectorResource` keeps only the error message, so a caller can't tell "not configured" from "rejected" or "unreachable". *Prefactor (ticket 01):* move `categoryName` to a small module and make `useCollectorResource` also return the API error code.
3. **The sidenav has no collector state, and the Collector page polls its own.** *Kept:* the quick action polls favorites status itself. The Favorites page signals saves and sync requests so the quick action refreshes at once. An App-level shared favorites store was rejected: it would change how the Favorites page gets its data and would break the Favorites and MissingFavoriteDownloads tests, which render the page on its own. The cost it would remove is one small extra status request every 5–30 s.
4. **Tests that render `<App />` with the sidebar answer every `fetch` with a catch-all, a strict router that throws on unknown paths, or a queue of responses.** A new sidenav request would break their assertions. The files are `App.test.tsx`, `galleries/Galleries.test.tsx`, `galleries/Reader.test.tsx` (its back-navigation test returns to the gallery detail page, which has the sidebar), `panda/PandaCatalog.test.tsx`, `libraries/SourceCleanup.test.tsx`, `libraries/LibraryRefresh.test.tsx`, `collector/Collector.test.tsx`, `collector/RawFeeds.test.tsx` and `collector/ReferenceImports.test.tsx`. *Kept:* ticket 03 stubs the quick action module in those nine files with `vi.mock`, as the library tests already do for `useScan`. Their assertions stay as they are.

## Behaviour

Terms: the *saved download categories* are the Panda automatic download categories in `downloads.categories` of the favorites status. Unsaved checkbox changes on the Favorites page are not part of them. The *tracked categories* are the saved download categories once the Panda favorite baseline is `ready`, and all ten categories before then.

Sidenav quick action:

1. The main sidenav shows a quick action below its links on every page that has the sidenav. A click starts an incremental Panda favorite sync of each saved download category. The request says nothing about categories; the collector reads the saved selection itself.
2. A click sends exactly one request. The collector admits all the categories in one transaction, so a click either requests every saved download category or shows one error.
3. With one saved download category, the button reads "Sync <category name>". Unnamed categories use "Category <n>", as the Favorites page does. With several, it reads "Sync favorites", and a tooltip (`title`) lists the category names.
4. Nothing is rendered until the first favorites status response arrives. When the response is `collector_not_configured`, nothing is rendered at all.
5. When the latest favorites status request fails for another reason, the button is disabled and shows a short reason in place of the state lines: "Collector rejected access" for `collector_unauthorized`, "Collector unreachable" for `collector_unavailable`, and "Sync status unavailable" for anything else, including Tana itself being unreachable. A later successful poll enables the button again.
6. With no saved download categories, the button is disabled and reads "Sync favorites". A hint reads "Check a category for auto-download on the Favorites page". "Favorites page" links to `#/collector/favorites`.
7. Before the Panda favorite baseline is `ready`, a click starts or resumes the baseline sync of all ten categories, as the Favorites page does today. A hint reads "Starts the baseline sync of all ten categories." While the baseline is `collecting`, a line reads "Baseline: <n> of 10 categories".
8. The button stays enabled while tracked categories are running, queued or waiting for cooldown. A click then succeeds (202), and the collector coalesces it with the existing work: a running or queued sync stays the only sync for its category, and a queued full re-sync stays full.
9. While its request is pending, the button reads "Requesting…" and is disabled. After success, the quick action fetches favorites status again at once.
10. When the request fails, an alert under the button shows the error: "The request could not be confirmed. Check collector status and try again." for `collector_unavailable`, the existing credentials message for `collector_unauthorized`, and "No category has auto-download checked. Check one on the Favorites page." for `no_download_categories` (the selection was cleared after the last poll). The quick action then fetches favorites status again. The alert clears on the next click.
11. An activity line shows the state of the tracked categories, taking the first that applies:
    - "Syncing" when any tracked category is running.
    - "Waiting for cooldown until <time>" when any is waiting for cooldown. The time is the earliest `retry_at` among the waiting ones.
    - "Queued" when any is queued.
    - "Never synced" when any tracked category has never completed a sync.
    - Otherwise, "Last synced <time>" with the oldest `last_synced_at` among the tracked categories.
12. A failure line shows when any tracked category is idle with last outcome `failed`. With one tracked category it reads "Failed: <error>". With several and one failed, it reads "<name> failed: <error>". With several failed, it reads "<n> categories failed: <error of the lowest-numbered one>". The line goes away once that category is queued or running again.
13. The quick action polls favorites status every 5 s while any tracked category is running or queued and no category is waiting for cooldown. It polls every 30 s otherwise, including for the whole time any category waits for cooldown. The collector runs one favorite sync at a time and waits out the cooldown first, so nothing changes before the cooldown ends.
14. When the Favorites page saves download categories or a sync request from it succeeds, the quick action fetches favorites status again at once. Checking or unchecking an auto-download box without saving changes neither the quick action's label nor the categories a click syncs.
15. Nothing starts a sync without a click: rendering and polling only read status (ADR 0010).
16. The Collector link keeps opening `#/collector` (Overview). It does not remember the last collector page.
17. Loading a page with the sidenav doesn't load the Collector page's code. The Collector page still loads lazily on its first visit.

Favorites page:

18. The Category dropdown and the Incremental/Full re-sync mode switch are gone, along with the "Sync favorites" button.
19. The table always lists all ten categories, one row each, and every row has a "Sync" button that starts an incremental sync of that category. Empty categories are no longer collapsed into one "Categories 2–4 · Show N empty" row. The status column header reads "Status", so it no longer collides with the row buttons.
20. Each category row has a small menu next to its "Download missing…" button. Its trigger is labelled "More actions for <category name>", and it holds one item, "Full re-sync", which requests a full re-sync of that category. "Download missing…" stays a button. From left to right, a row's actions are "Sync", "Download missing…" and the menu trigger.
21. A page-level "Sync all" button starts an incremental sync of all ten categories. The page offers no full re-sync of all categories as one action.
22. The success notices name the category: "Sync requested for <name>.", "Sync requested for all ten categories.", and "Full re-sync requested for <name>.". When the row had an incremental sync queued, the full re-sync notice adds " It replaces the queued sync.". When the row had an incremental sync running, including one waiting for cooldown (state `running` or `waiting_cooldown` with `full` false), it adds " It starts after the running sync.". Before the baseline is `ready`, every sync control shows "Baseline sync requested for all ten categories." instead.
23. The help text next to "Sync all" reads "Until the baseline is complete, any sync collects all ten categories without downloading favorites." before the baseline is `ready`. Afterwards it reads "Sync collects the newest favorites since the last sync. Full re-sync, in each category's menu, also removes favorites no longer on Panda; collected gallery references and metadata are kept." The baseline footer's collecting text ends with "Use Sync all to resume failed categories."
24. While a sync request is pending, while the collector is disconnected, or while the status is stale, every row "Sync", every "Full re-sync" item and "Sync all" are disabled. A failed request shows its error as an alert, as today.

Collector and local service:

25. `POST /api/collector/favorites/download-categories/sync` with body `{}` returns 202 after the collector has admitted the saved download categories. It returns 409 `no_download_categories` when none are saved, including before the baseline is ready, and then queues nothing. It returns 502 `collector_unauthorized` when the collector rejects Tana's token, 502 `collector_unavailable` when the collector can't be reached or fails, 503 `collector_not_configured` without a collector, and 400 for any other body.

## Design

**Data flow.** The browser talks only to the local service. The quick action polls `GET /api/collector/favorites/status`, an existing route that goes through `collectorapi.Client.FavoritesStatus` to the collector's status service. A click posts to the new local route. The route calls `collectorapi.Client.SyncDownloadCategories`, which posts to the new collector route, which calls `favorites.Service.EnqueueDownloadCategories`. The Favorites page keeps using the existing `POST /api/collector/favorites/{category|all}/sync` with `{"full": bool}`.

### Collector: `internal/collector/favorites`

- `var ErrNoDownloadCategories = errors.New("no_download_categories")`
- `func (s *Service) EnqueueDownloadCategories() error` admits an incremental sync of each saved download category. In one transaction it takes the download-settings row lock, as `store.enqueue` already does first, reads the saved categories, and returns `ErrNoDownloadCategories` if there are none. Otherwise it continues exactly like `Enqueue(category, false)` for that list. Before the baseline is `ready`, that bootstraps or resumes all ten categories. After that, it coalesces running and queued work and resumes failed checkpoints. It wakes the worker, and returns `ErrClosed` after `Close`, like `Enqueue`. Because the saved selection is read under the settings lock, a concurrent `SetDownloadSettings` can't interleave with it. How the transaction is shared with `store.enqueue` is the implementer's choice; the lock order (settings row before `favorite_syncs`) must hold.
- Test seam: package tests on `pgtest`, using the helpers in `downloads_test.go` (`manualService`, `configureDownloads`, `bootstrap`).

### Collector: `internal/collector/httpapi`

- `func HandleSyncDownloadCategories(service *favorites.Service) http.Handler`, registered as `POST /api/favorites/download-categories/sync`. The literal segment is more specific than `{category}`, so the mux accepts both patterns.
- Request: `{}`, decoded with `server.DecodeJSON` into `struct{}`. Unknown fields give 400 `invalid_json`.
- Responses: 202 with an empty body; 409 `{"error":"no_download_categories"}`; 503 `{"error":"<message>"}` for other errors, as `HandleSyncFavorites` does.
- Test seam: exercised end to end through the local httpapi test, which runs the real collector handler behind `httptest.NewServer`.

### `internal/collectorapi`

- `var ErrNoDownloadCategories = errors.New("no_download_categories")`
- `func (c *Client) SyncDownloadCategories(ctx context.Context) error` posts `{}` with the same 4 s timeout as `SyncFavorites`. 202 returns nil; 409 returns `ErrNoDownloadCategories`; any other status returns `*HTTPError{StatusCode}`.

### Local: `internal/local/httpapi`

- `func HandleCollectorSyncDownloadCategories(client *collectorapi.Client) http.Handler`, registered as `POST /api/collector/favorites/download-categories/sync`, next to the existing sync route in `collector.go`.
- Responses: 202 `{"status":"accepted"}`; 409 `{"error":"no_download_categories"}`; 502 `collector_unauthorized` when the upstream returns 401 or 403, otherwise 502 `collector_unavailable`; 503 `collector_not_configured` when the client is nil; 400 for a body other than `{}`.
- Test seam: `collector_test.go`, with the real collector handler behind `httptest.NewServer`, as in `TestLocalCollectorStatusAndSync`.

### Web

- `web/src/collector/favoriteCategories.ts` (prefactor) exports `categoryName(category: FavoriteCategory): string`. `Favorites.tsx` and `MissingFavoriteDownloads.tsx` use it.
- `web/src/collector/useCollectorResource.ts` (prefactor) also returns `errorCode: string`. It holds the `APIError.code` of the latest failed load, or `''`, and clears on success.
- `web/src/collector/api.ts` gains `syncDownloadCategories(signal: AbortSignal): Promise<void>`, which posts `{}` to the new local route, and `collectorMessages.no_download_categories`.
- `web/src/collector/favoriteSync.ts` holds pure logic, with no React components:
  - `favoriteSyncView(status: FavoritesStatus): FavoriteSyncView` returns the label, the tracked categories, whether a click starts the baseline, the no-categories hint, the activity (kind plus time), the failure text, the baseline progress, and whether to poll fast (behaviour 13: a tracked category is `running` or `queued`, and no category in `status.categories` is `waiting_cooldown`).
  - `notifyFavoritesChanged(): void`, plus a subscribe function or hook the quick action uses to refetch.
  - Test seam: a table of `FavoritesStatus` cases in `favoriteSync.test.ts`.
- `web/src/collector/FavoriteSyncAction.tsx` (default export, no props) is the quick action. It uses `useCollectorResource(getFavoritesStatus, true, 0, interval)`, `useMutation`, `favoriteSyncView`, `categoryName`, `formatDate`, `collectorLink` and Radix Themes. **`App.tsx` imports it statically, so it must never import `Collector.tsx` or any collector page component (Favorites, Overview, Downloads, …), whether directly or through other modules.** Otherwise the whole Collector chunk ends up in the main bundle. `collector/api.ts` is safe to import: it imports only `../api` and a type. Test seam: `FavoriteSyncAction.test.tsx`, rendering the component inside `Theme` with a stubbed `fetch`.
- `App.tsx` renders `<FavoriteSyncAction />` inside `.sidebar`, after the main `<nav>`. `App.css` gets the quick action styles: full sidebar width on desktop, and a compact row under the link row at ≤640px. Test seam: `App.test.tsx`, which stubs the module with a marker component that renders a fixed text, so the placement is visible without the real component's requests. The other eight files in **Fit**, item 4, may stub it with a component that renders `null`.
- `RowActions.tsx` gains an optional `trailing?: ReactNode`. When it is set, it is rendered in the delete position, in place of the delete button or the empty `deleteSlot` spacer. Existing callers pass no `trailing` and render unchanged.
- `Favorites.tsx`:
  - Drops the form card (Select, SegmentedControl, "Sync favorites") and the `category` and `full` state.
  - In its place, a `Card` holds a solid "Sync all" button followed by the help text from behaviour 23. They wrap onto two lines at narrow widths.
  - Drops the empty-category collapse: `emptyCategories`, `emptySynced`, `emptyCompleted`, `collapsible`, `showEmpty`, `categoryRanges` and the collapsed row. Every category in `status.categories` gets a row.
  - Each row's actions are `RowActions` with `slot1Width={80}`, where `slot1` is a soft gray "Sync" button at full slot width, and `slot2Width={160}`, where `slot2` is the existing "Download missing…" button. `trailing` is a `DropdownMenu`. Its trigger is a soft gray size-2 `IconButton` with `DotsHorizontalIcon` and `aria-label="More actions for <categoryName>"`. Its content is aligned to the end and holds one `DropdownMenu.Item`, "Full re-sync". The trigger is 32px wide (`var(--space-6)`), so the actions take 80 + 8 + 160 + 8 + 32 = 288px.
  - The actions column grows from 190 to 320 (`columns={[undefined, 100, 190, 150, 140, 320]}`), and the `TableCard` `minWidth` grows from 900 to 1030.
  - All sync controls share the existing `syncMutation` and `sync(category, full)` path.
  - Test seam: `Favorites.test.tsx`.

**Risky choices.**
- A new route instead of N per-category requests. It removes partial failure, and it makes the collector, which owns the saved selection, decide what a click syncs.
- The quick action reads only favorites status and infers connection problems from its error code. This saves a second poll of `/api/collector/status`.
- The quick action polls on its own, beside the Collector page. See **Fit**, item 3.

## Assumptions

- **One atomic route for the quick action, reading the saved categories on the collector.** Rejected: one browser request per category, which can partly fail and acts on the browser's possibly stale copy of the selection. Also rejected: changing the existing route to take a category list, which still trusts the browser's copy.
- **The new route returns 409 `no_download_categories` when none are saved, even before the baseline is ready.** Rejected: starting the baseline anyway. The quick action is disabled with no categories, and "Sync all" on the Favorites page still starts the baseline.
- **Hidden or disabled state comes from the favorites status error code.** 503 `collector_not_configured` hides the quick action, 502 `collector_unauthorized` shows "Collector rejected access", and anything else disables it. Rejected: also polling `/api/collector/status`.
- **The sidenav polls on its own: every 5 s while tracked categories are running or queued, every 30 s otherwise. The Favorites page sends an in-page change signal.** Rejected: an App-level shared favorites store (see Fit, item 3).
- **Any category waiting for cooldown drops the poll to the idle rate, even while tracked categories are queued.** The collector's single favorites worker claims the running sync first and waits out its `retry_at` before claiming anything else (`favorites.Service.run`, `NextSync`). So nothing changes until the cooldown ends, and an authenticated cooldown can last hours. Once the cooldown ends, the line flips within one idle poll. Rejected: fast polling while a category waits, which would poll every 5 s for hours in every open tab. Also rejected: scheduling a poll at `retry_at`, which adds a timer to save at most 30 s.
- **Before the baseline is ready, the label still follows the saved categories.** A hint and the "Baseline: n of 10" line say that a click syncs all ten, and the state lines track all ten. Rejected: a separate "Sync baseline" label.
- **Activity precedence is syncing, then waiting for cooldown, then queued, then never synced or last synced (oldest).** Failures get their own line, so activity elsewhere can't hide them. Rejected: one combined state, and the newest success time, which would hide a stale category.
- **The general label is "Sync favorites", with the names in a tooltip.** Rejected: "Sync auto-download categories", which wraps in the 176px sidebar.
- **Nothing renders before the first status response.** This avoids a flash on an unconfigured install.
- **Short reason texts in the sidenav: "Collector unreachable", "Collector rejected access", "Sync status unavailable".** Rejected: the long `collectorMessages` texts, which are written for the Collector page.
- **The row menu holds only "Full re-sync" and sits beside the unchanged "Download missing…" button.** Rejected: moving "Download missing…" into the menu. The brief doesn't ask for it, and it would churn the MissingFavoriteDownloads tests.
- **Full re-sync needs no confirmation dialog.** The notice says when it replaces a queued incremental sync or follows a running one (`Enqueue` upgrade semantics). A sync waiting for cooldown counts as running, because the status reports a running sync with a future `retry_at` as `waiting_cooldown`, and `Enqueue` queues the full re-sync as its follow-up.
- **Interpretation of the brief sentence "All ten categories stay listed in the table. Each row gets a 'Sync' button": the table always shows ten rows, and the empty-category collapse goes.** With the dropdown gone, the row is the only place to start a single-category sync, and a collapsed row would hide that button behind a click. Ten rows fit on one screen. Rejected: keeping the collapse, which makes the sentence true only after "Show N empty". Also rejected: a sync action on the collapsed row, which would sync several categories in non-atomic requests (the trap on the sync endpoint).
- **Row actions, left to right: "Sync" (80px), "Download missing…" (160px), then the menu trigger (32px). `RowActions` gains `trailing`.** "Sync" is the most frequent action, so it comes first, and the menu sits beside "Download missing…", as the brief asks. Rejected: a new row-actions component for Favorites only, which would duplicate the slot alignment `RowActions` already provides.
- **The status column header is renamed from "Sync" to "Status".** Rejected: keeping it, which would put a "Sync" column beside "Sync" buttons.
- **Notices name categories by `categoryName`.** Rejected: "category <n>".
- **Glossary.** "Quick action" and "auto-download" are UI words for a Panda favorite sync and a Panda automatic download category; no new `CONTEXT.md` term is needed.

## Out of scope

- A pinned-category setting, or any quick-action configuration beyond the saved Panda automatic download categories.
- Scheduled or automatic syncs. ADR 0010 stays in force.
- Remembering the last collector page in the Collector link.
- A full re-sync from the quick action, and a full re-sync of all categories as one action.
- Sharing one favorites-status poll between the sidenav and the Collector page.
- Showing per-page sync progress (entries saved, pages) in the sidenav.

## Manual checks

- **Sidenav layout on desktop.** The quick action sits below the four links inside the 176px sidebar. A long single category name truncates or wraps without overflowing. The activity and failure lines wrap cleanly, and a long error doesn't push the layout.
- **Phone width (≤640px).** The quick action collapses to one compact row under the link row, and the header grows by at most one row. The button stays tappable (44px target).
- **Poll timing against a real collector.** Click the quick action. The line moves through Queued, Syncing and "Last synced <time>" within about one 5 s poll of each change. When idle, it settles back to 30 s polling, seen in the network panel. Tune `ACTIVE_POLL_MS` (5000) and `IDLE_POLL_MS` (30000) in `FavoriteSyncAction.tsx`.
- **Polling during a cooldown.** With a category waiting for cooldown (an authenticated cooldown, or a retry after a transient error) and other tracked categories queued, the network panel shows favorites status requests at the idle rate (30 s), not every 5 s. Within about one idle poll after the cooldown ends, the line flips from "Waiting for cooldown until <time>" to "Syncing", and fast polling resumes. If the delay after the cooldown feels too long, tune `IDLE_POLL_MS`, or consider scheduling a poll at `retry_at`.
- **Collector outage and recovery.** Stop the collector. Within one idle poll, the quick action shows "Collector unreachable" with the button disabled. Start the collector again, and within one poll the button is enabled with its state lines back. With a wrong API token, it shows "Collector rejected access".
- **Click feedback.** While the request is in flight, the button reads "Requesting…" and is disabled; on a slow collector, this is visible. Afterwards the state moves to Queued or Syncing without a page reload, and an earlier error alert clears on the next click.
- **Refresh after a Favorites-page change.** Save a new auto-download selection, or click a row "Sync". The sidenav label or state updates at once, not after the idle poll.
- **Authenticated cooldown.** With an authenticated cooldown active (Panda ban, or reset it afterwards), the quick action shows "Waiting for cooldown until <time>", matching the Favorites page.
- **Bundle split.** Run `npm run build` in `web/`. The Collector page is still a separate lazy chunk. The main chunk grows by only a few kB and contains no Collector page code; for example, search it for "Reference imports" or "Download missing".
- **Favorites row actions.** All ten rows are listed. "Sync", "Download missing…" and the menu trigger line up across all ten rows and don't overflow the actions column. The menu opens beside its trigger and works by keyboard. The table still scrolls horizontally at narrow widths. Tune the slot widths (80, 160), the actions column width (320) and the table `minWidth` (1030) in `Favorites.tsx`.
