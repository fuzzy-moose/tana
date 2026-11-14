# 01: Prefactor: share favorite category names and collector error codes outside the Collector pages

What to build: nothing the user can see changes. This ticket prepares two things for the sidenav quick action (tickets 03 and 04). That component is imported statically by `App.tsx`, so it must not import the lazy Collector page modules. And it has to tell "collector not configured" apart from "rejected" and "unreachable".

- Move `categoryName(category: FavoriteCategory): string` out of `web/src/collector/Favorites.tsx` into a new module, `web/src/collector/favoriteCategories.ts`. Keep its behaviour: `category.name || \`Category ${category.category}\``. Have `Favorites.tsx` import it. Also replace the inline copy in `MissingFavoriteDownloads.tsx` (`const name = category.name || ...`) with it.
- Make `useCollectorResource` (`web/src/collector/useCollectorResource.ts`) also return `errorCode: string`. It holds the `code` of the `APIError` (`web/src/api.ts`) thrown by the latest failed load, or `''` when the error has no code. It clears to `''` on the next successful load, as `error` does. Leave every existing field and caller unchanged.

See spec **Fit**, item 2, and **Design → Web**.

Blocked by: None

Status: resolved

- [x] `web/src/collector/favoriteCategories.ts` exports `categoryName`. `Favorites.tsx` and `MissingFavoriteDownloads.tsx` use it and define no copy of their own.
- [x] `useCollectorResource` returns `errorCode`. After a load fails with an `APIError` whose code is `collector_not_configured`, `errorCode` is `'collector_not_configured'`. After the next successful load it is `''`.
- [x] Every existing web test passes with no test file edited. `npm run lint` and `npm run build` pass.

## Resolution

Added `web/src/collector/favoriteCategories.ts` (`categoryName`), used by `Favorites.tsx` and `MissingFavoriteDownloads.tsx`; `useCollectorResource` now returns `errorCode` from the latest failed load's `APIError`, covered by `useCollectorResource.test.ts`.
