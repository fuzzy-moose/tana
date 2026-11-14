# 05: Panda cards mark score override tags

What to build: a card of the Panda listing marks each tag that has a score override. The badge shows an open eye before the value for "never hide" and a crossed-out eye for "always hide", and it is green for "never hide" and red for "always hide", whatever weight the tag has. With ticket 04, the card in "Filter off" and "Hidden only" thus shows which tag hides or keeps the gallery. The cards of the local gallery list keep today's look. This ticket builds on the `override` field that spec **Design** fixes, and its tests use their own catalog pages, so it waits only for ticket 03. `scoreRuleNames` from ticket 01 is there by then, because ticket 03 waits for ticket 01.

This is spec behaviour 14, the web side of behaviours 16 and 18, and it must keep behaviours 19 and 22 to 24. Its parts are specified in spec **Design → Web** (the parts marked ticket 05):

- `web/src/panda/api.ts`: `PandaSummaryTag.override?: ScoreRule`, with the type from `web/src/panda/score.ts`.
- `web/src/galleries/GalleryBox.tsx`: `GalleryBoxTag.mark?: ReactNode`, rendered inside the badge before the value. The key that mounts `SummaryTags` again covers each tag's term and whether it has a mark, so the strip fits again when a mark arrives or goes. Since ticket 01, a strip that stays mounted fits again only on a resize, so without the mark in the key a wider badge would overflow the strip unnoticed. The effect that observes the strip keeps its dependency from ticket 01: a new array of the same tags and marks must not fit the strip again.
- `web/src/panda/PandaCard.tsx`: for a tag with `override`, `color` is `green` for `never_hide` and `red` for `always_hide`, and `mark` is `EyeOpenIcon` or `EyeNoneIcon` from `@radix-ui/react-icons`, followed by visually hidden text: the rule's name from `scoreRuleNames` (`score.ts`) and ": ". The override wins over the weight.
- `web/src/galleries/Galleries.css` may get a rule that keeps the icon from shrinking when a long value is cut (Hint: `flex-shrink: 0` on the badge's `svg`). The height rule of `.gallery-summary-tags` does not change.
- `web/src/galleries/GalleryCard.tsx`, `PandaDetails` and `web/src/panda/PandaCatalog.tsx` need no change.

Test seams: new cases in `web/src/panda/PandaCard.test.tsx`, which renders `<Theme><PandaCard /></Theme>`. A Radix `Badge` shows its colour as the attribute `data-accent-color` on its element (class `rt-Badge`). The case for the key uses a `ResizeObserver` stub that records `observe`, as ticket 01's case in `web/src/galleries/GalleryBox.test.tsx` does, at either seam. Existing assertions stay unchanged, among them ticket 01's case for the strip's observer and ticket 03's case for the local gallery card. The height of a badge with an icon and the cut of a long value are manual checks (spec **Manual checks**).

Blocked by: 03

Status: resolved

- [x] A `PandaCard` tag with `override: 'always_hide'` shows a list item whose text is "Always hide: " followed by the value, with one `svg` before the value in a badge with `data-accent-color` `red`. With `override: 'never_hide'` the text is "Never hide: " followed by the value, and the badge is `green`.
- [x] The `svg` of an "always hide" tag and the `svg` of a "never hide" tag have different markup.
- [x] A tag with `override: 'never_hide'` and `weight: -1.5` has a `green` badge, and a tag with `override: 'always_hide'` and `weight: 1.5` has a `red` badge.
- [x] A tag without `override` shows no `svg` and no rule name: its item's whole text is the value, and its badge has the colour its weight gives it (ticket 03's cases pass unchanged).
- [x] A `PandaCard` whose `summary_tags` put an override tag after another tag shows them in the given order: the browser does not sort.
- [x] Rendering a card again with the same tags in the same order, one of which has gained a mark, fits the strip again: a `ResizeObserver` stub that records its calls sees the list "Tags" observed once more. The same holds when a tag loses its mark. Rendering the card again with a new array of the same tags and marks causes no further `observe`.
- [x] The gallery detail page, the evaluation page and the Panda lookup page behave as before: their existing tests (`GalleryDetail.test.tsx`, `PandaScorePage.test.tsx`, `score.test.ts`, `PandaLookup.test.tsx`) pass unchanged. A card of the local gallery list keeps today's look: ticket 03's case for it passes unchanged.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`PandaSummaryTag.override` (api.ts) and `GalleryBoxTag.mark` (GalleryBox.tsx, rendered before the value and covered by the `SummaryTags` key); `PandaCard` gives an override tag its rule colour, an `EyeOpenIcon`/`EyeNoneIcon` and the hidden rule name; a CSS rule keeps the badge icon from shrinking; new cases in `PandaCard.test.tsx`.
