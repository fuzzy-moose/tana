# Score-ranked tags on Panda cards

## Problem

Each card of the Panda listing shows below its cover the first six tags of the gallery in namespace order, all gray. With an active Panda score model, the tags that decide the Panda gallery score are often not among them: a strong tag after the sixth is never offered to the card. The tags that do show look alike, whether the model weighs them up, down or not at all. A tag with a score override, which alone hides or keeps the gallery, has no place of its own and no mark. To learn why a gallery scores as it does, the user hovers over one card after the other.

## Solution

With an active Panda score model, the strip below the cover shows the tags that move the gallery's score most, the strongest first, liked and disliked together. A clearly liked tag is green, a clearly disliked tag is red, and the others stay gray. A tag with a score override comes before all of them, with an icon for "never hide" or "always hide", so in "Filter off" and "Hidden only" the card shows which tag keeps or hides the gallery. The collector ranks the tags when it builds the catalog page, so the cards need no further request. Without a model and without tag overrides, the cards look as they do today.

## Fit

Fits after one small prefactor ticket (ticket 01), which removes the two workarounds of items 7 and 8. Parts of the design that exist only because of how the code is built today:

1. **`catalog.Service.List` reads the score filter before it calls `summarizeTags`**, in one transaction, and `summarizeTags` already reads every tag of the page's galleries and keeps the first six. *Fits:* the summary lookup takes that filter and ranks before it cuts. It reads the same rows as today, plus one weight probe per tag.
2. **`markWeights` keeps five weights per item and mixes uploader and category with the tags**, for the hover details. *Kept as two lookups:* the strip has another selection (tags only, six, override tags first, tags without a weight included), and the hover details are out of scope. `markScores` and `markWeights` do not change.
3. **`gallerysearch.SummaryTagLimit` and its comment are shared** by the collector catalog and the local gallery list. *Fits:* the constant stays shared at 6. Its comment states only the limit, and the collector's lookup documents its own selection.
4. **`CatalogItem.SummaryTags` has the type `gallerysearch.TagSuggestion`**, which search completions also use. *Fits, with a new type:* `collectorapi.CatalogTag` carries the weight and the override, so only catalog items get them. One existing test literal changes its type and keeps its values (`TestCatalogProjectionUpdatesAndRawTagCompletions`).
5. **One existing assertion pins the selection that this feature replaces**: the last loop of `TestAppliedModelHidesGalleriesBelowTheThreshold` requires that gallery 104's summary tags hold no `female` tag. *Replaced in ticket 02* by the ranked expectation. No other existing assertion changes.
6. **`SummaryTags` in `GalleryBox.tsx` is shared with `GalleryCard`** of the local gallery list and renders each badge gray with the value only. *Fits:* a tag of `GalleryBox` gets two optional presentation fields (a badge colour, a mark). `PandaCard` fills them from the weight and the override. `GalleryCard` passes none, so it keeps today's look, and `GalleryBox` never learns what a weight or an override is.
7. **`SummaryTags` observes the strip's size in an effect that depends on the `tags` array itself.** Today every caller passes the array of the loaded page, and `saveVerdict` keeps that array when it patches a card. `PandaCard` will build a new array on every render, with a colour and a mark per tag. Every render of the listing, such as the one after a verdict save, would then replace the observer of every card. A new observer reports at once, so every strip that had dropped tags would fit again from all its tags. *Prefactor (ticket 01):* the effect depends on the number of tags, so no caller has to keep the array's identity. Rejected as a workaround: `PandaCard` memoizes its mapped tags.
8. **The names of the two override rules, "Always hide" and "Never hide", are a private constant of `PandaScorePage.tsx`**, and the mark on a card needs them. Their type `ScoreRule` is already exported from `score.ts`. *Prefactor (ticket 01):* `score.ts` exports the names beside the type, and the evaluation page reads them there. `PandaSummaryTag.override` uses `ScoreRule` as it is. Rejected as a workaround: a second copy of the names and of the rule values in the card's files.
9. **`SummaryTags` fits the strip when it mounts and when the strip's size changes**, and `GalleryBox` mounts it again only when the tags' terms change. A mark makes a badge wider. *Kept as an addition (ticket 05):* the key also covers whether each tag has a mark.
10. **`saveVerdict` patches only `verdict` and `displayed`** of the loaded card and loads nothing. *Fits:* behaviour 19 holds without new code in `PandaCatalog.tsx`. The tags stay because the patch keeps them, and no strip fits again because of the prefactor of item 7. A test pins each.
11. **The local `HandleCollectorCatalog` decodes the collector's page into `collectorapi.CatalogResult` and encodes it again.** *Fits:* the new fields are on the shared struct, so they pass without handler code.
12. **`List` reads the active model first and the weights in later statements** of the same transaction. An apply or "Turn off" between the two can give one page whose tags carry no weight. *Kept:* the hover details already behave so, and the next load is right.
13. **Tana and the collector are deployed independently.** *Kept:* a page from a collector without this feature has no `weight` and no `override` on its tags, and the cards then look as today.

## Behaviour

Terms: the *listing* is the Panda catalog page in Tana (`#/panda`) in any view or search. The *strip* is the area below a card's cover that shows tags. A strip *fits* when it measures its tags and drops those from the end that have no room. A *weighted tag* is a tag of the gallery that has a weight in the active Panda score model. An *override tag* is a tag of the gallery that has a score override. *Today's order* is namespace, then value, as the catalog page orders tags today. The score filter is *in force* when a model is active or at least one score override exists.

Order of the tags:

1. With an active Panda score model, the strip orders a gallery's weighted tags by the absolute value of their weight, the strongest first. Tags with a positive and with a negative weight rank together: the sign plays no part in the order.
2. Weighted tags with the same absolute weight keep today's order among themselves.
3. Tags without a weight follow the weighted tags in today's order, so a gallery with few or no weighted tags still shows tags.
4. The strip shows tags only. The weights of the uploader and of the Panda gallery category get no badge and do not change the order. They stay in the hover details.
5. The ranking reads all tags of the gallery: a weighted tag that today's order puts after the sixth place gets the place its weight gives it.
6. The catalog page offers a card at most six tags, as today: the first six of the ranked order. The strip keeps its size, shows the offered tags that fit, and drops tags from the end, so the strongest stay.
7. The order is the same in every view (All, Unseen, Interesting, Not for me) and in every score filter state (Filter on, Filter off, Hidden only).
8. A gallery with provisional Panda metadata ranks the tags that the feed gave. After collection, the collected tags decide.

Direction colours:

9. A weighted tag with a weight of 0.2 or more is green. A weighted tag with a weight of -0.2 or less is red.
10. A weighted tag with a weight between -0.2 and 0.2 is gray, like a tag without a weight. The badge text is the tag's value in every case: the colour alone tells the direction.
11. A gray weighted tag keeps its place by weight, before the tags without a weight. The browser shows the tags in the order of the catalog page and never sorts them.

Score override tags:

12. A tag with a score override comes before all weighted tags and all other tags, with or without an active model.
13. Among override tags, "never hide" comes before "always hide", as in the rule order of the score filter. Tags with the same rule keep today's order.
14. An override tag has a mark before its value: an open eye for "never hide", a crossed-out eye for "always hide". Assistive technology reads the rule name before the value ("Never hide: glasses"). The badge is green for "never hide" and red for "always hide", whatever weight the tag has.
15. Only overrides for tags count. An override for an uploader or a Panda gallery category changes no strip.
16. In "Filter off" and "Hidden only", the card thus shows which tag hides or keeps the gallery: that tag is at the start of the strip with its mark. When a verdict, an uploader or category override, or the score decides, the hover details say so, as today.

Without a model:

17. Without an active Panda score model and without tag overrides, the card shows today's tags in today's order, all gray. This also holds when only uploader or category overrides exist.
18. With tag overrides only, the override tags come first with their mark. The other tags follow in today's order, gray.

State changes:

19. A verdict change never changes the tags of a card: giving, changing or removing a verdict leaves the tags, their order, colours and marks as loaded, and loads nothing. It fits no strip again, on that card or on any other card of the page.
20. A model apply, a threshold change, "Turn off" or an override change takes effect on the cards with the next load of the listing. No card on a loaded page changes its tags.

Failures:

21. The ranked tags come with the catalog page. There is no new request and no new route, so there is no new failure: a collector that is unreachable or not configured fails the listing as today.

What stays as it is:

22. The local gallery list and the gallery detail page do not change: alphabetical gray tags, and no dependency on the collector. A card of the local gallery list never gets a colour or a mark from the score model.
23. The hover details, the evaluation page and the Panda lookup page do not change: they show what they show today.
24. The strip never changes its height, with or without colours and marks, so the page capacity of the listing, which is measured from the card size, stays the same.

## Design

**Data flow.** `catalog.Service.List` ranks the tags of the page's items and puts them, with weight and override, into `CatalogItem.SummaryTags`. The collector's `GET /api/catalog` writes the page. The local `GET /api/collector/catalog` reads it through `collectorapi.Client` into the same struct and writes it to the browser. `PandaCatalog` hands each item to `PandaCard`, which turns weight and override into a badge colour and a mark for `GalleryBox`. No route, no migration and no stored value is added. ADR 0019 holds: the collector computes the order per catalog page and stores no score per gallery.

Signatures, payloads and the stated constraints are fixed. Query shapes, private helpers and test sequences are the implementer's. A line that starts with "Hint:" is optional.

### `internal/collectorapi` (tickets 02 and 04)

```go
// CatalogTag is a tag offered to a card of the catalog listing.
type CatalogTag struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
	Term      string `json:"term"`
	// Weight is the tag's weight in the active Panda score model.
	Weight *float64 `json:"weight,omitempty"`
	// Override is the rule of the tag's score override.
	Override string `json:"override,omitempty"`
}
```

Ticket 02 adds the type with `Weight`, and ticket 04 adds `Override`. `CatalogItem.SummaryTags` becomes `[]CatalogTag` and keeps the JSON name `summary_tags`. `Override` holds `ScoreRuleAlwaysHide` or `ScoreRuleNeverHide`. A tag without weight and override marshals to exactly today's object (`namespace`, `value`, `term`). `gallerysearch.TagSuggestion` does not change.

### `internal/collector/catalog` (tickets 02 and 04)

`List` passes the score filter it has read to the summary lookup. The lookup stays one statement over the page's items (`gt.gallery_id = ANY(<page ids>)`), never over every candidate, in every view and every filter state.

- **Filter not in force:** today's result. Each item gets its first `gallerysearch.SummaryTagLimit` tags in today's order, without `Weight` and `Override`.
- **Filter in force:** all tags of each gallery are ordered by these keys, and the first `SummaryTagLimit` are kept:
  1. the group: first the tags with a tag score override (ticket 04), then the tags with a weight in the active model and no override, then all other tags;
  2. in the first group, "never hide" before "always hide";
  3. in the second group, the larger absolute weight first;
  4. today's order.
- **Fields:** `Override` is the rule for a tag of the first group. `Weight` is the tag's weight in the active model for every tag that has one, the first group included. A weight of 0 is a weight. A tag of the third group has neither.
- **Today's order is the database's order** `ORDER BY t.namespace, t.value_lower`. The ranking uses it for key 4, for example with a stable sort over rows read in that order, or with the whole order in SQL. A string comparison in Go is not the same order under the database's collation.
- **Weights** come from `panda_score_weights` of the filter's model with `kind = 'tag'`, by `tag_id`. The unique index `panda_score_weight_tag (model_id, tag_id) WHERE kind = 'tag'` serves the probe. Hint: probe the weight of each tag in a scalar subquery, as `scoreSum` does, so the plan does not depend on the statistics of a model that was just applied.
- **Overrides** come from `scoreFilter.overrides`, which `List` has read already: the entries of kind tag carry `TagID` and `Rule`. No further statement reads them.
- Uploader and category weights and overrides play no part.
- An item without tags keeps an empty, non-nil `SummaryTags`, which the page writes as `[]`: the browser maps over it.
- `markScores`, `markWeights`, `scoreItemWeights` and `decidingOverride` do not change.

`internal/gallerysearch/search.go`: the comment on `SummaryTagLimit` states only the limit ("bounds the tags offered to a listing card, which shows those that fit"). The comment on the collector's summary lookup states its selection. `internal/local/gallery/browse.go` needs no change.

Test seam: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `projectFeed`, `setVerdict`, `setOverride`, `scorePage`, `itemByID`, `unseenPage`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate` and `weightOf`.

### HTTP (`internal/collector/httpapi`, `internal/local/httpapi`)

No route is added, and the handlers need no code: `HandleCatalog` and `HandleCollectorCatalog` carry the new fields through the shared struct.

Test seam: `internal/local/httpapi/catalog_test.go`, through the real collector handler, as `TestLocalCatalogScoreFilter` does.

### Web

`web/src/panda/score.ts` (ticket 01):

```ts
/** The names of the score override rules, as the user reads them. */
export const scoreRuleNames: Record<ScoreRule, string> = { always_hide: 'Always hide', never_hide: 'Never hide' }
```

`PandaScorePage.tsx` reads the names from there and keeps no copy of its own. The evaluation page shows what it shows today.

`web/src/panda/api.ts`:

```ts
export interface PandaSummaryTag extends TagSuggestion {
  weight?: number
  override?: ScoreRule // the type from './score'
}
```

`PandaGallery.summary_tags` becomes `PandaSummaryTag[]`. Ticket 03 adds the type with `weight`, and ticket 05 adds `override`.

`web/src/galleries/GalleryBox.tsx`:

```ts
export interface GalleryBoxTag extends TagSuggestion {
  /** Badge colour; gray without one. */
  color?: BadgeColor // the type of the Radix Badge's `color` prop
  /** Shown inside the badge, before the value. */
  mark?: ReactNode
}
```

`GalleryBoxProps.tags` becomes `GalleryBoxTag[]`. Ticket 03 adds the type with `color`, and ticket 05 adds `mark`.

- `SummaryTags` renders the tags in the order it gets, each as today's badge with `color` (gray by default), `mark` and the value. The fit logic, the list's accessible name "Tags" and the CSS height of `.gallery-summary-tags` do not change.
- **When a strip fits.** A strip fits when `SummaryTags` mounts and when the strip's size changes, and at no other time. From ticket 01 on, the effect that observes the strip's size depends on the number of tags and not on the `tags` array, so a caller may pass a new array on every render. Re-rendering a card whose tags, their order and their marks have not changed, as after a verdict save, must not fit its strip again: nothing in `SummaryTags` may depend on the identity of the array or of a value in it.
- **The key.** `GalleryBox` mounts `SummaryTags` again when its key changes. The key covers each tag's term, in the given order, and from ticket 05 on whether the tag has a mark. From ticket 01 on, a strip that stays mounted fits again only on a resize, so every change that alters the width of a badge must change the key. A colour alters no width.
- `web/src/galleries/GalleryCard.tsx` needs no change. It passes the local list's tags, which have no `color` and no `mark`.

`web/src/panda/PandaCard.tsx` maps each `PandaSummaryTag` to a `GalleryBoxTag`:

- with `override` (ticket 05): `color` is `green` for `never_hide` and `red` for `always_hide`. `mark` is `EyeOpenIcon` for `never_hide` and `EyeNoneIcon` for `always_hide` (`@radix-ui/react-icons`), followed by visually hidden text: the rule's name from `scoreRuleNames` and ": ", which gives "Never hide: " or "Always hide: ". The override wins over the weight.
- else with `weight` (ticket 03): `green` at `weight >= clearTagWeight`, `red` at `weight <= -clearTagWeight`, otherwise no colour. `clearTagWeight = 0.2` is a constant in this file.
- else: no colour and no mark.

`PandaCard` never sorts or cuts the tags. It may build the mapped array during render: ticket 01 makes that safe. `PandaDetails` and `PandaCatalog.tsx` (with `saveVerdict` and the sizing card) need no change. The gallery detail page, the evaluation page and the Panda lookup page behave as before.

Test seams: `web/src/panda/PandaCard.test.tsx` renders `<Theme><PandaCard /></Theme>`. A Radix `Badge` shows its colour as `data-accent-color` on its element (class `rt-Badge`). The local card is tested through `<App />` at `#/galleries` (`web/src/App.test.tsx` or `web/src/galleries/Galleries.test.tsx`), the verdict case in `web/src/panda/PandaVerdicts.test.tsx`. Whether a strip is observed again is tested on `<Theme><GalleryBox /></Theme>` in a new `web/src/galleries/GalleryBox.test.tsx`, with a `ResizeObserver` stub that records `observe` and `disconnect` (`vi.stubGlobal`, as `Galleries.test.tsx` does; the stub of `web/src/testSetup.ts` records nothing). jsdom has no layout, so the fit itself is a manual check.

## Assumptions

- **The bound for a clear weight is 0.2**, both ends included: a weight of exactly 0.2 or -0.2 is coloured. A weight is the logarithm of a frequency ratio, so 0.2 stands for a ratio of about 1.22. Rejected: colouring every weight that is not zero (the brief asks for gray near zero), and a bound derived from the model's weight distribution (needs model statistics with every page; a constant is easy to tune).
- **Green is positive, red is negative.** They match the verdict controls: the accent colour of "Interesting" is green, and a pressed "Not for me" is red. Rejected: blue and orange, which a colour-blind user tells apart more easily. The hover details still list the strongest weights with their sign.
- **The colour is the only sign of direction.** Rejected: a "+" or "-" before the value, or hidden text for the direction. A prefix takes width, so fewer tags fit.
- **The collector sends the weight, and the browser picks the colour.** Rejected: the collector sends a direction. The bound is a matter of presentation and belongs beside the colours.
- **An override tag takes the colour of its rule, not of its weight**, and its mark is an icon. Rejected: the weight's colour with the icon (a red "never hide" tag reads as a contradiction), and a gray badge with the icon (too easy to miss in "Hidden only").
- **"Never hide" comes before "always hide"** in the strip, because "never hide" wins in the score filter. Rejected: plain today's order among override tags.
- **Ties keep today's order**, among weighted tags and among override tags of one rule.
- **An override tag also carries its weight** when it has one, so `weight` always means the same. The card does not use it.
- **A new type `collectorapi.CatalogTag`.** Rejected: more fields on `gallerysearch.TagSuggestion`, which search completions share.
- **The strip's lookup is separate from `markWeights`.** Rejected: a larger `scoreItemWeights` with the strip derived from `Score.Weights`. That list mixes uploader and category, holds no tag without a weight, and is part of the hover details.
- **Only tag overrides touch the strip**, and with only uploader or category overrides the cards look as today.
- **The strip's observer depends on the number of tags, by a prefactor in `GalleryBox.tsx`.** Rejected: `PandaCard` memoizes the mapped tags on `gallery.summary_tags`. That keeps a hidden rule (the `tags` of `GalleryBox` must keep their identity between renders), and the next caller that maps its tags breaks it again without a failing test.
- **The rule names are shared as `scoreRuleNames` from `score.ts`, and `override` has the type `ScoreRule`.** Rejected: a copy of the two names in `PandaCard`, which can drift from the evaluation page. The export follows `verdictLabels` in `api.ts`, which `PandaCard` and `PandaCatalog` share.
- **One prefactor ticket holds both prefactors.** Each is a few lines in `web/`, and both keep what the user sees. Rejected: two tickets, each with its own review of a few lines, and steps inside tickets 03 and 05, which would mix a refactoring of the evaluation page and of the shared card into feature tickets.
- **No version check between Tana and the collector.** The user normally updates both together. A page without the new fields gives today's cards.
- **No new term in `CONTEXT.md`.** "Strip", "fits", "weighted tag", "override tag", "today's order" and "mark" are words of this spec for presentation. Weight, score override and score filter are used as `CONTEXT.md` defines them. No ADR is added or changed.

## Out of scope

- The local gallery list and the gallery detail page.
- A badge for the uploader or the Panda gallery category in the strip.
- Changes to what the hover details, the evaluation page and the Panda lookup page show.
- Another tag limit, another strip height, or tags that work as links.
- A legend for the colours and marks, and a setting for the bound.
- An update of loaded cards when the model or an override changes elsewhere.
- Stored ranks or scores per gallery.

## Manual checks

- **The bound.** Open the listing with the real active model and look at several pages. Good: the green and red tags are the ones the user would name as liked and disliked, and a card rarely shows six coloured tags. Tune `clearTagWeight` (0.2) in `web/src/panda/PandaCard.tsx`.
- **Strip height and page capacity.** Compare the listing with and without an active model at the same window size. Good: every card has the same height in both, the grid shows the same number of cards per page, and no strip shows a third row or a cut badge.
- **Fit.** Narrow the window until a strip cannot hold six tags. Good: the tags at the end drop and the strongest stay, and widening the window brings them back.
- **Verdicts on trimmed strips.** Narrow the window until most strips have dropped tags, then give, change and remove verdicts on several cards in a row. Good: every click answers at once, and no strip on the page flickers or shows a dropped tag for a moment. A performance recording in the browser shows no layout work in the strips after a verdict save. There is no constant to tune: a strip that fits again here means that something in `SummaryTags` (`web/src/galleries/GalleryBox.tsx`) depends on the identity of its tags.
- **Marks.** Look at a card with a "never hide" tag and one with an "always hide" tag, also with a long tag value. Good: the badge with the icon is as high as the others, the two icons are easy to tell apart at card size, the value is cut with an ellipsis and the icon stays whole.
- **A mark that arrives on a loaded page.** Narrow the window until strips have dropped tags. Give a tag that stands first on a card an override in a second tab, then switch the first tab between "Filter on" and "Filter off". Good: the strip shows the mark and fits again, with no cut badge and no tag that vanishes without the strip dropping it from the end.
- **Colours in the dark theme.** Good: green, red and gray badges are told apart at a glance, on a normal card and on the card of a seen gallery.
- **"Filter off" and "Hidden only".** With an "always hide" tag override, open both states. Good: every card that the override hides starts its strip with that tag and its mark.
