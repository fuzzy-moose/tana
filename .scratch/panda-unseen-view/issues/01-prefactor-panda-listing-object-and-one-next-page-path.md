# 01: Prefactor: build Panda listing links from one listing object, and leave a page through one next-page path

What to build: nothing the user can see changes. This ticket prepares `web/src/panda/PandaCatalog.tsx` for the unseen view. Ticket 05 adds a sixth listing flag (`unseen`), and ticket 03 records the page whenever the user goes to the next page.

- `web/src/navigation.ts`: export `interface PandaListing { search: string, cursor: string, includeExpunged: boolean, categories: string[], bypassDefault: boolean }`. Change `pandaListingLink` to `pandaListingLink(listing: Partial<PandaListing>): string`. It writes the same parameters in the same order as today (`q`, `cursor`, `include_expunged`, each `category`, `bypass_default`), each only when set, so every link stays byte-for-byte the same. The panda route becomes `{ kind: 'panda' } & PandaListing`, with the same parsing as today.
- `web/src/panda/api.ts`: change `listPandaCatalog` to `listPandaCatalog(listing: PandaListing, pageSize: number, signal?: AbortSignal)`, building the same query string as today.
- `web/src/panda/PandaCatalog.tsx`: props become `PandaListing`, and `App.tsx` passes the route's listing fields. Build every link (`pageHref`, the clear link, category change, default-filter save, bypass toggle, "Include expunged") from the listing object with the changed fields overridden. Keep the effects depending on the individual fields: `useRoute` memoizes `categories` by value, and an object built during render must not become an effect dependency, or every render refetches.
- One function moves to the next page. The ArrowRight/`d` handler calls it. The Next link keeps its `href` and `rel="next"`, and gets an `onClick` that, for a plain primary click without Ctrl, Meta, Shift or Alt, cancels the browser's own link navigation and calls the same function. Other clicks keep the native link behaviour.

See spec **Fit**, items 6 and 7, and **Design → Web**.

Blocked by: None

Status: resolved

- [x] `navigation.ts` exports `PandaListing` and `pandaListingLink(listing: Partial<PandaListing>)`. No caller passes positional arguments, and `listPandaCatalog` takes `(listing, pageSize, signal)`.
- [x] A plain primary click on Next doesn't trigger the browser's own link navigation, and still moves the hash to the next page, through the same next-page function that ArrowRight and `d` call. The Next element is still a link whose `href` is the next page.
- [x] Every existing web test passes with no test file edited, including the exact catalog request URLs and listing hashes asserted in `PandaCatalog.test.tsx`. `npm run lint` and `npm run build` pass.

## Resolution

`PandaListing` in `web/src/navigation.ts` now drives `pandaListingLink`, the panda route, `listPandaCatalog` and the `PandaCatalog` props and links. A single `goNext` in `PandaCatalog.tsx` serves ArrowRight/`d` and plain primary clicks on the Next link, covered by a new test in `PandaCatalog.test.tsx`.
