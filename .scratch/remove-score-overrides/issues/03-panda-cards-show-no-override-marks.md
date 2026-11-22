# 03: Panda cards show no override marks

What to build: a card of the Panda listing draws no eye mark and no override colour, and its hover details name one reason for a hidden gallery. A tag that had an override looks like any other tag: the pin mark when it is pinned, else the star of a favorite author, and the colour of its weight. Spec behaviours 10 and 11 (mark and colour); shape in spec **Design → Catalog page**.

The card stops using `scoreRuleNames` and `ScoreRule` of `web/src/panda/score.ts`; ticket 05 removes those exports. The order of the tags stays the catalog page's.

Blocked by: None

Status: resolved

- [x] A tag with `source: 'pin'` shows the pin mark with the text "Pinned: " and a badge in the colour of its weight. A favorite author's tag without a pin shows the star.
- [x] The hover details of a gallery with `rule: 'below_threshold'`, on a page with a score filter, say "Hidden: the score is below the threshold." Without that rule, or on a page without a score filter, they name no reason.
- [x] `PandaSummaryTag` and `PandaGalleryScore` have no `override`, `rule` is `'below_threshold'` only, and `npm run build` passes.
- [x] `npm test` and `npm run lint` pass in `web/`.

## Resolution

`PandaCard.tsx` drops the eye marks, override colours and override reasons (tags mark a pin, else a favorite author's star, coloured by weight; details name only "Hidden: the score is below the threshold."); `api.ts` drops `override` from `PandaSummaryTag` and `PandaGalleryScore` and narrows `rule` to `'below_threshold'`; card tests cover pins and the single reason.
