# 01: Prefactor: A re-rendered card keeps its tag strip's fit, and the score rule names are shared

What to build: nothing the user sees changes. Two small changes in `web/` remove what tickets 03 and 05 would otherwise work around (spec **Fit**, items 7 and 8, and the parts of spec **Design → Web** marked ticket 01).

- **The strip's fit does not depend on the identity of the tag array** (`web/src/galleries/GalleryBox.tsx`). `SummaryTags` observes the strip's size in an effect that depends on the `tags` array itself. A caller that passes a new array of the same tags thus gets a new observer on every render. In a browser a new observer reports at once, so a strip that had dropped tags fits again from all its tags. From this ticket on, the effect depends on the number of tags, and a new array of the same tags leaves the observer in place. Hint: the observer's callback reads only `tags.length`, so that is the effect's dependency. The strip still fits when it mounts, when its size changes, and when `GalleryBox` mounts it again through its key. The key, the fit logic, the markup and `Galleries.css` stay as they are.
- **The names of the score override rules live beside their type** (`web/src/panda/score.ts`, `web/src/panda/PandaScorePage.tsx`). `score.ts` exports `scoreRuleNames: Record<ScoreRule, string>` with the values "Always hide" and "Never hide". `PandaScorePage.tsx` reads the names from there and keeps no copy of its own.

Test seams: a new test file `web/src/galleries/GalleryBox.test.tsx` renders `<Theme><GalleryBox /></Theme>` and renders it again. Hint: `vi.stubGlobal('ResizeObserver', ...)` with a class that records `observe` and `disconnect`, as `web/src/galleries/Galleries.test.tsx` stubs it. The stub of `web/src/testSetup.ts` records nothing. The second part needs no new test: `web/src/panda/PandaScorePage.test.tsx` already reads both rule names from the page. No existing test file changes.

Blocked by: None

Status: resolved

- [x] Rendering a `GalleryBox` again with a new `tags` array that holds the same tags leaves the strip's observer in place: a `ResizeObserver` stub that records its calls sees the list "Tags" observed by the first render, and no `observe` and no `disconnect` call from the second.
- [x] A card of the local gallery list shows its tags as before: `web/src/App.test.tsx`, which reads the list "Tags" of a card, passes unedited.
- [x] The evaluation page shows "Always hide" and "Never hide" on its override buttons, in its rule control and in its override list as before: `PandaScorePage.test.tsx` passes unedited.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, and no existing test file changes.

## Resolution

`SummaryTags` in `web/src/galleries/GalleryBox.tsx` now keys its observer effect on `tags.length` (covered by the new `GalleryBox.test.tsx`), and `web/src/panda/score.ts` exports `scoreRuleNames`, which `PandaScorePage.tsx` reads instead of its own copy.
