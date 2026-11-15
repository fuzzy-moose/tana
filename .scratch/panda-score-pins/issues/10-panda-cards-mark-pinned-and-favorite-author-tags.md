# 10: Panda cards mark pinned and favorite-author tags

What to build: a card of the Panda listing marks the tags that the user weighs by hand. A tag whose weight is a pin has a pin icon before its value. The tag of a favorite author has a star, whether the author bonus or the model gives its weight, unless it is pinned: then it shows the pin. Both keep the colour that their weight gives them. A tag with a score override keeps its eye and shows no second mark. The hover details name the source beside each weight: "(pinned)" or "(author bonus)". The card offers no pin action. The collector sends the `source` since ticket 04 and `favorite_author` since ticket 07. This ticket builds on the fields that spec **Design** fixes, and its tests use their own galleries, so it waits for no other ticket.

This is spec behaviours 25, 26 and 28, and the web side of behaviours 24, 27, 29 and 30. Its parts are specified in spec **Design → Web**, **Cards (ticket 10)**:

- `web/src/panda/score.ts`: `export type ScoreSource = 'pin' | 'author_bonus'`.
- `web/src/panda/api.ts`: `PandaSummaryTag.source?: ScoreSource`, `PandaSummaryTag.favorite_author?: boolean` and `PandaScoreWeight.source?: ScoreSource`.
- `web/src/panda/PandaCard.tsx`:
  - `stripTag` gives a tag at most one mark, the first that applies: the override's mark for a tag with `override`, as today. Else `DrawingPinFilledIcon` followed by visually hidden "Pinned: " for a tag with `source: 'pin'`. Else `StarFilledIcon` followed by visually hidden "Favorite author: " for a tag with `favorite_author`, whatever its `source` is (`@radix-ui/react-icons`). A pinned tag and a favorite author's tag get the colour of their weight by `clearTagWeight`, as a tag without a mark does.
  - `PandaDetails`: a weight line ends with " (pinned)" for `source: 'pin'` and with " (author bonus)" for `source: 'author_bonus'`.
  - The card never sorts or cuts the tags, and it gets no new control.
- `web/src/galleries/GalleryBox.tsx`, `web/src/galleries/Galleries.css` and `web/src/panda/PandaCatalog.tsx` need no change: the key of `SummaryTags` already covers whether a tag has a mark, so the strip fits again when a pin mark or a star arrives or goes.

Test seam: new cases in `web/src/panda/PandaCard.test.tsx`, which renders `<Theme><PandaCard /></Theme>` and `<PandaDetails />` and reads the strip with its helper `stripTags`. Existing assertions stay unchanged. The look of the icons at card size is a manual check (spec **Manual checks**).

Blocked by: None

Status: resolved

- [x] A tag with `source: 'pin'` shows "Pinned: " before its value, with one icon before the value. Its badge has the colour of its weight: green for a clearly positive pin, red for a clearly negative one, gray for a pin of 0.
- [x] A tag with `favorite_author` shows "Favorite author: " before its value, with one icon that differs from the pin's, in the colour of its weight. It shows the star with `source: 'author_bonus'` and without a `source`.
- [x] A favorite author's tag with `source: 'pin'` shows the pin mark and no "Favorite author: ".
- [x] A tag with an override shows only the override's mark, also with `source: 'pin'` or `favorite_author`.
- [x] A tag without `source`, `favorite_author` and `override` shows no icon, and its text is its value.
- [x] `PandaDetails` ends the line of a weight with " (pinned)" for a pinned tag, uploader or category and with " (author bonus)" for a tag whose weight the bonus gives. A line of a calculated weight is as today.
- [x] A card with a pinned tag and without `onVerdict` renders no button.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, with existing assertions unchanged.

## Resolution

`stripTag` in `web/src/panda/PandaCard.tsx` marks pinned tags with a pin and favorite authors' tags with a star (override first, then pin, then star), colouring both by weight; `PandaDetails` suffixes " (pinned)" or " (author bonus)"; `ScoreSource` and the `source`/`favorite_author` fields were added to `score.ts` and `api.ts`, with new cases in `PandaCard.test.tsx`.
