# 01: Prefactor: one view field for the Panda listing

What to build: nothing the user can see. The Panda listing holds its view as one field, `view`, instead of the boolean `unseen`, so that later tickets can add the verdict views without a state that says "unseen and Interesting". Links, catalog requests and the UI stay exactly as they are.

This is spec **Fit** item 1. Its parts are specified in spec **Design → Web** (the lines marked ticket 01):

- `web/src/navigation.ts`: `export type PandaView = 'all' | 'unseen'`. `PandaListing.view: PandaView` replaces `unseen: boolean`. The route parses `unseen=true` as `view: 'unseen'` and everything else as `view: 'all'`. `pandaListingLink` writes `unseen=true` as the last parameter for `'unseen'`, and nothing for `'all'` or a missing view.
- `web/src/panda/api.ts`: `listPandaCatalog` appends `unseen=true` for `view === 'unseen'`, as today.
- `web/src/App.tsx` passes `view={route.view}`.
- `web/src/panda/PandaCatalog.tsx`: props, the listing object, `pageHref`, `requestKey`, `clearHref`, `allHref`, the effects and the view control read and write `view`. Everything that tests `unseen` today tests `view === 'unseen'`. The view control's `onValueChange` writes the chosen value as `view`.

Keep the behaviour unchanged and leave every existing test file unedited. The tests only observe hashes, requests and the DOM, so they pin this ticket.

Blocked by: None

Status: resolved

- [x] `PandaListing` has a `view` field of the exported type `PandaView` and no `unseen` field, and no file under `web/src` outside tests reads a listing's `unseen` field (`npm run build` type-checks this).
- [x] At `#/panda?unseen=true` the view control shows "Unseen" selected and the catalog request ends with `&unseen=true`. At `#/panda` it shows "All" and the request has no `unseen` parameter.
- [x] Every listing link in the unseen view (search, category, default bypass, "Include expunged", "Clear all filters", Next, "Newest galleries") still ends with `unseen=true`, and "Show all galleries" still drops it.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with every existing test file unedited.

## Resolution

`PandaListing.view: PandaView` (`'all' | 'unseen'`) replaces `unseen` in `web/src/navigation.ts`; `api.ts`, `App.tsx` and `PandaCatalog.tsx` read and write `view`, with links, requests and tests unchanged.
