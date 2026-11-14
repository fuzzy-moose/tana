# 04: Quick action shows the live sync state of the tracked categories

What to build: below the sidenav button, the user sees what the tracked categories are doing without opening the Collector. The line says whether they are syncing, queued, waiting for the authenticated cooldown, or synced, with the last success time. A separate line shows a failure and its error, and a progress line shows the baseline while it is collecting. The quick action polls faster while work is active. It refreshes at once when the user saves download categories or requests a sync on the Favorites page. Unsaved checkbox changes never reach it.

Spec behaviours 7 (the progress line), 11–14, and **Design → Web**:

- Extend `favoriteSyncView` in `web/src/collector/favoriteSync.ts`:
  - The tracked categories: the saved download categories once the baseline is `ready`, otherwise all ten.
  - The activity, by the precedence in behaviour 11.
  - The failure text, by behaviour 12.
  - The baseline progress: `Baseline: <n> of 10 categories` from `downloads.baseline_categories` while `collecting`.
  - A fast-poll flag (behaviour 13): true when any tracked category is `running` or `queued` and no category in `status.categories` is `waiting_cooldown`. A waiting category, tracked or not, keeps the flag false. The collector's single favorites worker waits out that cooldown before it claims anything else, and an authenticated cooldown can last hours.
  Extend the table test in `favoriteSync.test.ts`.
- `FavoriteSyncAction.tsx` renders the activity line ("Syncing", "Queued", "Waiting for cooldown until <time>", "Never synced", "Last synced <time>"; times through `formatDate`), the failure line, and the progress line. It polls every `ACTIVE_POLL_MS = 5000` while the fast-poll flag is set, and every `IDLE_POLL_MS` otherwise. The status lines are replaced by the disabled reason while the latest status request fails (ticket 03).
- Add `notifyFavoritesChanged()` to `favoriteSync.ts`, with a way to subscribe; the mechanism is the implementer's choice, e.g. a `window` event or a module-level listener set. The quick action refetches when it fires. In `web/src/collector/Favorites.tsx`, call it after a successful `save` and after a successful `sync` (in their `onSuccess`).

Blocked by: 03

Status: resolved

- [x] `favoriteSyncView` returns the activity by precedence. One tracked category running and another failed gives "Syncing" plus a failure. A waiting category with `retry_at` T plus a queued one gives "Waiting for cooldown until" T. Queued only gives "Queued". When one tracked category has no `last_synced_at`, it gives "Never synced". When all have one, it gives "Last synced" with the oldest `last_synced_at`.
- [x] `favoriteSyncView` returns the failure text by behaviour 12: "Failed: <error>" for a single tracked category, "<name> failed: <error>" for one of several, and "<n> categories failed: <error>" for several, using the lowest-numbered category's error. It returns no failure text once that category is `queued` or `running` again.
- [x] Before the baseline is `ready`, the tracked categories are all ten, whatever the saved selection. While `collecting` with `baseline_categories: 4`, the progress reads "Baseline: 4 of 10 categories".
- [x] The fast-poll flag is true when a tracked category is `running` or `queued` and no category is waiting. It is false in each of these cases: a tracked category is `waiting_cooldown` and another tracked one is `queued`; an untracked category is `waiting_cooldown` and a tracked one is `queued`; the only active category is untracked; nothing is active.
- [x] The rendered quick action shows the activity line and, when present, the failure line with the error text.
- [x] With `<FavoriteSyncAction />` and `<Favorites />` rendered together, saving a new download-category selection on the Favorites page updates the quick action's label without waiting for its poll interval.
- [x] Checking an auto-download box on the Favorites page without saving leaves the quick action's label unchanged.
- [x] `npm test`, `npm run lint` and `npm run build` pass.

## Resolution

`favoriteSyncView` now returns the tracked categories, activity, failure text, baseline progress and fast-poll flag; `FavoriteSyncAction` renders them and polls at 5 s or 30 s; `notifyFavoritesChanged`/`subscribeFavoritesChanged` let Favorites save and sync refresh it at once; `useCollectorResource` takes an opt-in `rescheduleOnIntervalChange` (used by the quick action) that reschedules its pending poll on an interval change instead of loading at once, which stays the default for Collector pages.
