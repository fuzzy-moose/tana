# 03: Sidenav quick action syncs the saved download categories

What to build: from any page that has the main sidenav, the user clicks one button below Galleries, Panda, Libraries and Collector to start an incremental Panda favorite sync of their saved Panda automatic download categories. The button names the category when only one is saved. It hides when no collector is configured. It is disabled, with a short reason, when the collector is unreachable or rejects access, or when no category is saved. Before the baseline is ready, it says that a click starts the baseline sync of all ten categories. This ticket covers the button and its click. The live state lines and the faster polling come in ticket 04.

Spec behaviours 1–10 and 15–17, and **Design → Web**:

- `web/src/collector/api.ts`: add `syncDownloadCategories(signal: AbortSignal): Promise<void>`, which posts `{}` to `/api/collector/favorites/download-categories/sync` with `collectorMessages`. Add `collectorMessages.no_download_categories = 'No category has auto-download checked. Check one on the Favorites page.'`.
- `web/src/collector/favoriteSync.ts`: start the pure `favoriteSyncView(status: FavoritesStatus)` with the label, the saved categories, whether a click starts the baseline, and the no-categories hint. Ticket 04 extends it. Pin it with a small table test in `favoriteSync.test.ts`.
- `web/src/collector/FavoriteSyncAction.tsx` (default export, no props) polls with `useCollectorResource(getFavoritesStatus, true, 0, IDLE_POLL_MS)`, where `IDLE_POLL_MS = 30000`. It uses `errorCode` from ticket 01 for the hidden and disabled states, and `useMutation` for the click. After the click settles, it calls the resource's `refresh`. It must import neither `Collector.tsx` nor any collector page component, directly or through other modules (spec **Design**, bold note).
- `web/src/App.tsx`: render `<FavoriteSyncAction />` inside `<aside className="sidebar">`, after the main `<nav>`. `web/src/App.css`: styles for desktop and a compact row at ≤640px.
- Tests that render `<App />` on a route with the sidebar answer every `fetch` with a catch-all, a strict router that throws `Unexpected request` on unknown paths, or a queue of responses (`mockResolvedValueOnce`). The new sidenav request breaks them. Stub the quick action module in all nine of these files, with the import path adjusted, as `libraries/*.test.tsx` already do for `useScan`. Keep every existing assertion.
  - `App.test.tsx`: stub it with a marker component that renders a fixed text and no JSX, e.g. `vi.mock('./collector/FavoriteSyncAction', () => ({ default: () => 'Favorite sync action' }))`. The placement criterion below is checked against this marker. The first test's `expect(fetchMock).not.toHaveBeenCalled()` keeps holding, because the marker sends no request.
  - Stub it with `default: () => null` in these eight:
    - `galleries/Galleries.test.tsx`
    - `galleries/Reader.test.tsx`: its back-navigation test returns from the reader to `#/galleries/1`, which renders the sidebar, and its catch-all answers favorites status with gallery JSON.
    - `panda/PandaCatalog.test.tsx`
    - `libraries/SourceCleanup.test.tsx`
    - `libraries/LibraryRefresh.test.tsx`
    - `collector/Collector.test.tsx`: its `statusResponse` answers `/api/collector/favorites/status`, so the real quick action would add "Sync …" buttons and text to the sidebar.
    - `collector/RawFeeds.test.tsx`
    - `collector/ReferenceImports.test.tsx`

Blocked by: 01, 02

Status: resolved

- [x] When favorites status answers 503 `collector_not_configured`, the quick action renders nothing. Nothing is rendered before the first response either.
- [x] When favorites status answers 502 `collector_unauthorized`, the button is disabled and "Collector rejected access" is shown. With 502 `collector_unavailable` it shows "Collector unreachable", and with any other failure "Sync status unavailable".
- [x] With no saved download categories, the button reads "Sync favorites" and is disabled. The hint "Check a category for auto-download on the Favorites page" links to `#/collector/favorites`.
- [x] With saved categories `[1]` and category 1 named "Manga", the button reads "Sync Manga". With `[3]` and category 3 unnamed, it reads "Sync Category 3". With `[0, 1]`, it reads "Sync favorites", and its `title` lists both category names.
- [x] With the baseline `not_started` or `collecting` and at least one saved category, the button is enabled and the hint "Starts the baseline sync of all ten categories." is shown.
- [x] A click sends exactly one request, `POST /api/collector/favorites/download-categories/sync` with body `{}`, and then fetches favorites status again without waiting for the poll interval.
- [x] The button stays enabled, and a click still posts, while a saved category is running or queued.
- [x] When the click's request fails, an alert shows the mapped message: "The request could not be confirmed. Check collector status and try again." for `collector_unavailable`, and the `no_download_categories` message for 409.
- [x] Rendering and polling alone send no POST request.
- [x] In `App.test.tsx`, with the quick action stubbed by the marker, the marker's text appears inside the sidebar after the "Main navigation" `nav`, and the "Collector" link's `href` is still `#/collector`.
- [x] Every existing web test passes with its assertions unchanged. The only additions to existing test files are the module stubs in the nine files listed above and the placement check in `App.test.tsx`. `npm run lint` and `npm run build` pass.

## Resolution

Added `favoriteSync.ts` (`favoriteSyncView`), `FavoriteSyncAction.tsx` rendered after the main nav in `App.tsx` with `App.css` styles, `syncDownloadCategories` and the `no_download_categories` message in `collector/api.ts`, their tests, and the quick action stubs in the nine `<App />` test files.
