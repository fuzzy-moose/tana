# 03: Panda cards colour tags by direction

What to build: a card of the Panda listing colours the tags in its strip by the direction of their weight. A tag with a weight of 0.2 or more is green, a tag with a weight of -0.2 or less is red, and every other tag stays gray. The card shows the tags in the order of the catalog page. The cards of the local gallery list keep today's look. The weights reach the browser with ticket 02. This ticket builds on the `weight` field that spec **Design** fixes, and its tests use their own catalog pages, so it does not wait for ticket 02. It waits for ticket 01: `PandaCard` now builds a new tag array on every render, and only ticket 01 makes that safe for the strip.

This is spec behaviours 9, 10 and 19, the web side of behaviours 11, 17, 22 and 23, and it must keep behaviours 6 and 24 (the strip's fit and height, both manual checks). Its parts are specified in spec **Design → Web** (the parts marked ticket 03):

- `web/src/panda/api.ts`: `PandaSummaryTag` with `weight?` (no `override` yet), and `PandaGallery.summary_tags: PandaSummaryTag[]`.
- `web/src/galleries/GalleryBox.tsx`: the exported type `GalleryBoxTag` with `color?` (no `mark` yet), and `tags: GalleryBoxTag[]`. `SummaryTags` gives each badge the tag's colour, gray without one. The order, the fit logic, the key, the accessible name "Tags" and the badge text (the value) stay as they are.
- `web/src/panda/PandaCard.tsx`: the constant `clearTagWeight = 0.2` and the mapping from `weight` to `color`. `PandaCard` never sorts or cuts the tags. It may build the mapped array during render.
- `web/src/galleries/GalleryCard.tsx`, `web/src/galleries/Galleries.css`, `PandaDetails` and `web/src/panda/PandaCatalog.tsx` need no change. `saveVerdict` already patches only `verdict` and `displayed`, so behaviour 19 needs a test and no code.

One constraint from spec **Design → Web** ("When a strip fits"): re-rendering a card whose gallery has not changed must not fit its strip again. The listing renders every card again after each verdict save, and a card then passes a new array of the same tags. Ticket 01 made the strip indifferent to that, and its case in `web/src/galleries/GalleryBox.test.tsx` guards it. Keep it so: nothing in `SummaryTags` may depend on the identity of the `tags` array or of a value in it.

Test seams: new cases in `web/src/panda/PandaCard.test.tsx`, which renders `<Theme><PandaCard /></Theme>`. A Radix `Badge` shows its colour as the attribute `data-accent-color` on its element (class `rt-Badge`). The local card is tested through `<App />` at `#/galleries`, as `web/src/App.test.tsx` does. The verdict case renders `<App />` at `#/panda` with a stubbed `fetch`, as `web/src/panda/PandaVerdicts.test.tsx` does. Existing assertions stay unchanged, and new cases may be added to existing test files. jsdom has no layout, so the strip's fit and height are manual checks (spec **Manual checks**).

Blocked by: 01

Status: resolved

- [x] A `PandaCard` whose `summary_tags` carry the weights 0.9, -0.9, 0.2, -0.2, 0.19 and -0.19, in this order, shows six items in the list "Tags" in the same order, each with the tag's value as its whole text. Their badges have `data-accent-color` `green`, `red`, `green`, `red`, `gray`, `gray`.
- [x] A tag with the weight 0 and a tag without `weight` have a `gray` badge.
- [x] A `PandaCard` whose `summary_tags` put a weaker tag before a stronger one shows them in the given order: the browser does not sort.
- [x] A `PandaCard` whose tags have no `weight` renders one gray badge per tag with the value as its text, as before this ticket.
- [x] In the local gallery list, a gallery whose `summary_tags` JSON carries `"weight": 2` and `"override": "always_hide"` on a tag shows that tag with a `gray` badge whose whole text is the value.
- [x] In the Panda listing, after a verdict is saved on a card whose tags have weights, the card's tags have the same order and colours as before, and no catalog request follows.
- [x] The gallery detail page, the evaluation page and the Panda lookup page behave as before: their existing tests (`GalleryDetail.test.tsx`, `PandaScorePage.test.tsx`, `score.test.ts`, `PandaLookup.test.tsx`) pass unchanged.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged, among them ticket 01's case for the strip's observer.

## Resolution

`PandaSummaryTag` (with `weight`) in `web/src/panda/api.ts`, `GalleryBoxTag` (with `color`) in `GalleryBox.tsx`, whose badges take the tag colour (gray by default), and the `clearTagWeight` mapping in `PandaCard.tsx`; new cases in `PandaCard.test.tsx`, `PandaVerdicts.test.tsx` and `App.test.tsx`.
