# Verdicts for Panda galleries

## Problem

The Panda catalog knows which galleries the user has seen, but not what the user thought of them. A gallery the user rejected looks the same as one the user wanted to come back to, and there is no way to list either kind. The only way to keep a gallery in mind is outside Tana.

## Solution

Each card in the Panda listing has two controls on its cover, "Not for me" and "Interesting". A click gives the gallery that verdict, a click on the other control changes it, and a click on the given one removes it. The verdict shows on the card in every view. The view control in the Panda toolbar gains two views, "Interesting" and "Not for me", that list only the galleries with that verdict, combined with the search and every filter. The verdicts live on the collector with the seen state, so every device and browser tab sees the same state.

## Fit

Fits, with one small web prefactor (ticket 01). Parts of the design that exist only because of how the code is built today:

1. **`PandaListing` holds the view as `unseen: boolean`.** A verdict filter beside it could express "unseen and Interesting", which is always empty and which the listing must not offer. Every link builder would need a guard. *Prefactor (ticket 01):* one `view` field replaces the boolean, so the combination can't be written. The link format doesn't change.
2. **`GalleryBox` renders the whole card as one `<a>` that opens Panda, and it is also the card of the local listing.** A button inside the link is invalid, and its click opens Panda. *Kept, inside the component:* `GalleryBox` gains an optional `actions` slot that it renders beside the link, never in it. Without `actions` it renders exactly today's DOM, so the local listing doesn't change. This is an addition, not a prefactor: no existing caller changes.
3. **`useGalleryGrid` sets the page size from a hidden card (`sizingGallery`).** *Fits:* the controls are positioned over the cover and take no space in the card's flow. The sizing card renders without them.
4. **`GalleryBox` shows `PandaDetails` in a Radix tooltip whose trigger is the link.** *Fits:* the controls are outside the trigger, and the tooltip is placed outside the link's box, which contains the controls, so it never covers them. Radix keeps a tooltip open while the pointer moves from its trigger toward it, so the tooltip can stay open with the pointer on a control. That is accepted, and `GalleryBox` leaves its `Tooltip` as it is (see **Assumptions**). Keyboard focus that moves from the link to a control closes the tooltip.
5. **`PandaCatalog` loads the page again when its request key changes, and every in-flow alert changes the measured page capacity, which is part of the request key.** In the unseen view a new load removes the card that just got a verdict. *Workaround, kept:* a saved verdict patches the loaded page in component state and changes no part of the request key, and the failure message is laid out outside the page flow, unlike the catalog's other alerts. Not prefactored: making the page capacity independent of alerts means changing `useGalleryGrid` or the layout of the existing alerts in both listings, for a message that should be rare.
6. **The key handler in `PandaCatalog` ignores input fields but not buttons.** *Kept on purpose:* paging keys keep working with the focus on a verdict control (behaviour 12). The usual flow is to judge a few cards with the pointer and press `d`, and the focus is then on the last control clicked.
7. **`catalogPageQuery` reads the catalog in order and stops at the limit.** With a verdict condition it would scan millions of rows to find a few. *Fits:* the verdict views use their own page query that starts from the verdict table. It returns the same columns, so the rest of `List` (predicate, cursor, outer query, summaries) is shared.
8. **`useMutation` runs one request at a time and aborts it on unmount.** Several cards can save at once, and a save must survive a page change. *Kept:* each card tracks its own pending save, and the save request takes no abort signal, like `recordPandaDisplays`.
9. **The seen tables have no foreign key to catalog rows (ADR 0019).** *Fits:* the verdict table follows the same rule, so a verdict survives every projection change.
10. **`index.css` gives `.rt-IconButton` a 44px minimum height on coarse pointers, but no minimum width.** *Kept:* the verdict controls set the matching minimum width themselves.
11. **`PandaCatalog.test.tsx` answers every unknown `fetch` with a 200 catalog page.** *Kept:* no existing test clicks a verdict control, so none sends a save. New tests install their own handlers.
12. **Tana and the collector are deployed independently.** An older collector ignores `verdict` and returns 404 for the new routes. *Kept:* the user updates both together (see **Out of scope**).

## Behaviour

Terms: *listing* is the Panda catalog page in Tana (`#/panda`) in any view or search. *Verdict view* is the listing restricted to the galleries with one verdict. *Neutral* is a displayed Panda gallery without a verdict. *Page* is the set of cards the listing currently shows.

Verdict controls on a card:

1. Each card in each Panda listing (the view of all galleries, each search, the unseen view, and both verdict views) has two controls, "Not for me" and "Interesting". A gallery has at most one verdict.
2. The controls sit on the cover: "Not for me" in its top-left corner, "Interesting" in its top-right corner. They take no space in the card, so the card size and the number of galleries on a page don't change.
3. On a device with a hovering pointer, the controls show while the pointer is on the card or the keyboard focus is on the card's link or on one of its controls. Otherwise they are hidden, except a given verdict (behaviour 5).
4. On a touch screen the controls always show.
5. The control of a given verdict always shows, in its pressed look, in every view. Assistive technology gets it as a pressed toggle button named after the verdict.
6. A click on the control of the verdict the gallery doesn't have gives that verdict. With the other verdict given, the same click changes the verdict. A click on the control of the given verdict removes it.
7. The controls work by pointer and tap. They are buttons in the Tab order, after their card's link: "Not for me", then "Interesting". Enter and Space activate the focused control, as for every button. There is no selected card and no key that gives a verdict.
8. A click on a control never opens the gallery on Panda. A click on the cover between the controls opens it as before.
9. The details tooltip of a card never covers its controls.
10. There is no verdict for a full page, no reason with a verdict, and no verdict control on the Panda lookup page.
11. The local gallery listing does not change.
12. With the focus on a verdict control, ArrowRight, ArrowLeft, `a` and `d` change the page as everywhere on the listing, and going to the next page records the page as seen.

Verdict and seen state:

13. Giving or changing a verdict makes the gallery a displayed Panda gallery at once, with a display at the time of the verdict. The card gets the subdued look right after the save, in the view of all galleries, in searches and in the unseen view.
14. A verdict change never moves or removes a card on the current page, in any view. The listing does not load again after a save.
15. In the unseen view, the card stays in its position and shows the verdict until the user leaves the page. After any new load of the listing (a reload, "Reload results", a resize that changes the page size, a search or filter change) and on later pages, the gallery is absent from the unseen view.
16. In a verdict view, a card whose verdict the user changes or removes stays in its position until the user leaves the page. After a new load it is absent from that view.
17. A removed verdict leaves the gallery displayed. It stays out of the unseen view.
18. A catch-up does not give, change, or remove a verdict.

Verdicts in the listings:

19. A "Not for me" gallery stays in the view of all galleries and in each search, with its pressed "Not for me" control at full strength on the subdued card. No listing hides a gallery because of its verdict.
20. The view control in the Panda toolbar has four options: "All", "Unseen", "Interesting", "Not for me". "Interesting" lists only galleries with the verdict "Interesting", and "Not for me" only galleries with the verdict "Not for me".
21. The listing link holds the view: `verdict=interesting` or `verdict=not_for_me`, as the last parameter. Choosing a view drops the cursor. Reloading or sharing a link keeps its view. `#/panda` without `verdict` and `unseen` shows all galleries.
22. A verdict view combines with the search, the categories, the default filter and its bypass, and "Include expunged". Changing any of them keeps the view, and the collector applies all of them together. "Clear all filters" keeps the view. An expunged gallery with the verdict is listed only with "Include expunged", as in every listing.
23. A verdict view keeps the catalog order: Panda publication time (first sighting while unknown), newest first, then gallery ID.
24. A verdict view and the unseen view can't be combined. The view control selects exactly one view. A link that carries both `verdict` and `unseen=true` opens the verdict view. A link with an unknown `verdict` value ignores it.
25. A verdict view pages like the view of all galleries: Next and Previous follow the collector's cursors, and going to the next page records the page as every listing does. It has no "Catch up…" button.
26. Cards in a verdict view have the normal look, not the subdued one, and don't carry the word "Seen": every gallery there is seen.
27. An empty verdict view without a cursor and without a search, category or applied default filter reads "No Panda gallery has this verdict." with "Mark a gallery as Interesting on its card, and it appears here." (or "… as Not for me …"), and offers "Show all galleries" (the view of all galleries, no cursor).
28. An empty verdict view without a cursor but with a search, categories or an applied default filter reads "No Panda gallery with this verdict matches." with "Try different titles, tags, or categories, or clear all filters.", and offers "Clear all filters" (the same verdict view without search and filters, default filter bypassed).
29. An empty verdict view with a cursor shows the state the view of all galleries shows there: "No more matching Panda galleries." with "Newest galleries", which keeps the verdict view.
30. A listing shows the verdicts as of its last load, plus the saves made on it. A verdict given on another device or tab shows after the next load.

Favorites:

31. Panda favorites have no role. A card shows only a verdict that the user gave in Tana, and no part of this feature reads the `favorites` table.

Failures:

32. A card shows a verdict only after the collector stored it. While a save is pending, the card keeps its state from before the click, the clicked control shows a spinner, and both controls of that card ignore clicks. Other cards stay usable.
33. If a save fails, the card returns to its state before the click, and a message appears over the bottom edge of the card area: "Could not save the verdict for <gallery title>." followed by the error. The message is not a dialog. It takes no space in the page, moves no card, leaves the keyboard focus where it is, and doesn't block paging by key or by button. The listing doesn't load again. The message stays, also across pages of the listing, until the user removes it with "Dismiss", a later failure replaces it, a later save for the same gallery succeeds, or the user leaves the Panda catalog. When several saves fail, it shows the one that settled last.
34. A save goes out at once and is not cancelled by a page change or a reload right after the click. A save that settles after the user moved to another page of the listing still stores, and a failure still shows the message. After the user left the Panda catalog for another Tana page, a save still stores or fails on the collector but shows nothing.
35. If the collector is unreachable, a listing in any view shows the current error display of the Panda catalog: the stale-results alert, or "Panda catalog unavailable." on a fresh visit. A save made then fails as in behaviour 33.

Guards:

36. The verdicts are one shared state on the collector. Every device and browser tab reads and writes the same state, and it survives restarts of Tana and the collector.
37. The collector stores, per gallery, the gallery ID, the verdict and the time it was given. It stores no copy of the metadata. Changing the verdict to the other one sets the time anew. Giving the verdict the gallery already has keeps the time. Removing a verdict removes the row.
38. A verdict can be given to a gallery with provisional Panda metadata and stays after collected metadata replaces it.
39. A verdict survives every change of the catalog projection: provisional-to-collected replacement, tag replacement, removal and re-projection of the catalog row, and the gallery becoming expunged.
40. The verdict belongs to a gallery ID. A new version of a gallery with a verdict has its own gallery ID and no verdict.

Local API:

41. `GET /api/collector/catalog` accepts `verdict` (`interesting` or `not_for_me`; absent or empty means no verdict view) and forwards it to the collector. Any other value, and a verdict together with `unseen=true`, gives 400 `invalid_query`. Each item carries `verdict` when the gallery has one, in every view.
42. `PUT /api/collector/catalog/verdicts/{id}` with `{"verdict": "interesting"}` or `{"verdict": "not_for_me"}` returns 204. Another verdict gives 400 `invalid_verdict`, and an `{id}` that is not a positive integer gives 400 `invalid_gallery_reference`, both without calling the collector. A body with unknown fields gives 400 `invalid_json`. 502 `collector_unauthorized` when the collector rejects Tana's token, 502 `collector_unavailable` when it can't be reached or fails, 503 `collector_not_configured` without a collector.
43. `DELETE /api/collector/catalog/verdicts/{id}` returns 204, also when the gallery has no verdict. The errors are as in behaviour 42, without the verdict check.

## Design

**Data flow.** The browser talks only to the local service. Listing: `GET /api/collector/catalog?...&verdict=interesting` → `collectorapi.Client.Catalog` → collector `GET /api/catalog?...&verdict=interesting` → `catalog.Service.List`. Save: browser `PUT /api/collector/catalog/verdicts/{id}` → `collectorapi.Client.SetCatalogVerdict` → collector `PUT /api/catalog/verdicts/{id}` → `catalog.Service.SetVerdict`. Remove: the same path with `DELETE` → `RemoveCatalogVerdict` → `RemoveVerdict`. Non-test code in `internal/local/` reaches the collector only through `internal/collectorapi`.

### Collector schema (`internal/collector/storage/migrations`)

- `0004_panda_gallery_verdicts.sql` (ticket 02):
  ```sql
  CREATE TABLE panda_gallery_verdicts (
      gallery_id BIGINT PRIMARY KEY CHECK (gallery_id > 0),
      verdict TEXT NOT NULL CHECK (verdict IN ('interesting', 'not_for_me')),
      given_at TIMESTAMPTZ NOT NULL
  );
  ```
  A comment states why it has no foreign key, as `0002_panda_gallery_displays.sql` does: the verdict is the user's judgment, not a projection, and must survive every change of `catalog_galleries` (ADR 0019). No secondary index: the table holds the few galleries the user judged, and the verdict views read it whole.
- No sqlc query changes (the catalog package uses pgx directly), so `go generate` is not needed. `storage_test.go` counts the embedded migrations itself.

### Collector: `internal/collector/catalog`

New file `verdicts.go` (ticket 02):

- `var ErrInvalidVerdict = errors.New("invalid verdict")`
- `func (s *Service) SetVerdict(ctx context.Context, galleryID int64, verdict string) error`: `galleryID <= 0` returns `ErrInvalidGalleryReference`. A verdict that `collectorapi.ValidVerdict` rejects returns `ErrInvalidVerdict`. Otherwise it stores the verdict for the gallery (behaviour 37: a different verdict replaces the old one and sets `given_at` to the collector database's time; the same verdict leaves the row as it is). Every successful call also adds one display for the gallery at the collector database's time to `panda_gallery_displays`, with the conflict rule of `RecordDisplays`. Both happen in one transaction or one statement, so a gallery with a verdict is always displayed. It accepts every positive ID, with or without a catalog row, as `RecordDisplays` does.
- `func (s *Service) RemoveVerdict(ctx context.Context, galleryID int64) error`: `galleryID <= 0` returns `ErrInvalidGalleryReference`. It deletes the gallery's verdict row if there is one and touches no display.
- `List` (ticket 02) sets `CatalogItem.Verdict` for every item in every view, by looking up only the page's items by gallery ID, next to `markDisplayed`. The page query doesn't change in ticket 02.
- `CatchUp`, `RecordDisplays`, `Project` and `ProjectFeed` don't touch the verdict table.

`List` with `options.Verdict` (ticket 04):

- A value that is not empty and that `collectorapi.ValidVerdict` rejects returns `ErrInvalidQuery`. A verdict together with `options.Unseen` returns `ErrInvalidQuery`. Both are checked before the transaction starts.
- The page comes from a verdict page query instead of `catalogPageQuery`. It takes the same shared predicate on alias `g` (search, default filter, categories, expunged, cursor bound) and returns the same columns (`gallery_id`, `ordered_at`) in the same order, so the outer query, `summarizeTags`, `markDisplayed` and the verdict lookup run unchanged.
- **The verdict page query must start from the verdicts.** Shape: a `MATERIALIZED` common table expression joins `panda_gallery_verdicts` (filtered to the verdict, as a bound argument) to `catalog_galleries g` on the primary key and applies the predicate, with no `ORDER BY` and no `LIMIT` inside. The query around it sorts and limits. Keeping the limit outside the materialized join stops the planner from choosing an ordered scan of the catalog that probes the verdict table per row. The multi-category `UNION ALL` of `catalogPageQuery` is not needed here: the category conditions are already in the predicate.
- Paging follows the rules of the view of all galleries: next and previous cursors, and the recovery from an emptied page. Nothing of the unseen paging rules applies.
- A verdict whose gallery has no catalog row is not listed.
- Test seam: package tests on `pgtest` in a new `verdicts_test.go`, using `openCatalog`, `retain`, `projectFeed`, `assertCatalogIDs`, `unseenPage` and `displayedIDs` from the existing test files. Tests may read `panda_gallery_verdicts` directly for `given_at`. Behaviours 18 and 40 and the column list of behaviour 37 are given by the structure (no code path from `CatchUp` or the projection to the table, a table keyed by gallery ID, the migration above) and get no test of their own.

### `internal/collectorapi`

- `const VerdictInteresting = "interesting"`, `const VerdictNotForMe = "not_for_me"`, `func ValidVerdict(verdict string) bool` (ticket 02). The catalog package and the local handlers use them. The migration's check constraint names the same two values.
- `CatalogItem.Verdict string` with JSON `verdict,omitempty` (ticket 02).
- `func (c *Client) SetCatalogVerdict(ctx context.Context, galleryID int64, verdict string) error` sends `PUT /api/catalog/verdicts/{id}` with `{"verdict": verdict}` (ticket 02). 204 returns nil. Any other status returns `downloadResponseError(response)`, as `postCatalogSeen` does.
- `func (c *Client) RemoveCatalogVerdict(ctx context.Context, galleryID int64) error` sends `DELETE /api/catalog/verdicts/{id}`, with the same result handling (ticket 02).
- `CatalogOptions.Verdict string`, sent by `Catalog` as `verdict=<value>` on every request, empty included, like the other parameters (ticket 04).

### Collector: `internal/collector/httpapi`

- `func HandleSetCatalogVerdict(service *catalog.Service) http.Handler`, registered as `PUT /api/catalog/verdicts/{id}`. Body `{"verdict": "..."}` via `server.DecodeJSON`. 204 with no body on success (ticket 02).
- `func HandleRemoveCatalogVerdict(service *catalog.Service) http.Handler`, registered as `DELETE /api/catalog/verdicts/{id}`. 204 on success (ticket 02).
- An `{id}` that does not parse as an integer is handled as `catalog.ErrInvalidGalleryReference`.
- `catalogError` gains `ErrInvalidVerdict` → 400 `invalid_verdict`. `ErrInvalidGalleryReference` already maps to 400 `invalid_gallery_reference`.
- `HandleCatalog` passes `verdict` to `options.Verdict` unparsed (ticket 04). `List` validates it.
- Test seam: the validation tables in `catalog_test.go`. The success paths run end to end through the local httpapi tests.

### Local: `internal/local/httpapi`

- New file `catalog_verdicts.go` (ticket 02) with `func HandleCollectorSetCatalogVerdict(client *collectorapi.Client) http.Handler` and `func HandleCollectorRemoveCatalogVerdict(client *collectorapi.Client) http.Handler`, registered as `PUT` and `DELETE /api/collector/catalog/verdicts/{id}` next to the catalog routes in `routes.go`.
- Both check `catalogCollectorConfigured` first, then validate themselves before calling the client (behaviours 42 and 43): the ID with 400 `invalid_gallery_reference` (not `pathID`, which answers 404), the verdict with `collectorapi.ValidVerdict` and 400 `invalid_verdict`. Collector errors go through `writeCatalogCollectorError`, which needs no change.
- `HandleCollectorCatalog` passes `verdict` to `options.Verdict` unparsed (ticket 04). The collector's 400 `invalid_query` already passes through `writeCatalogCollectorError`.
- Test seam: `catalog_test.go`, running the real collector handler behind `httptest.NewServer`, as `TestLocalRecordCatalogDisplays` and `TestLocalCatalogCatchUp` do.

### Web

- `web/src/navigation.ts`:
  - Ticket 01: `export type PandaView = 'all' | 'unseen'`, and `PandaListing.view: PandaView` replaces `unseen: boolean`. The route parses `unseen=true` as `view: 'unseen'`, everything else as `'all'`. `pandaListingLink` writes `unseen=true` last for `'unseen'` and nothing for `'all'` or a missing view.
  - Ticket 05: `PandaView` gains `'interesting' | 'not_for_me'`. The route parses a `verdict` parameter with one of these two values as that view, before it looks at `unseen` (behaviour 24). `pandaListingLink` writes `verdict=<view>` last for a verdict view, and never together with `unseen`.
- `web/src/panda/api.ts`:
  - `listPandaCatalog` appends `unseen=true` for the unseen view (ticket 01, as today) and `verdict=<view>` last for a verdict view (ticket 05). Other views send neither, so existing URL assertions hold.
  - `export type PandaVerdict = 'interesting' | 'not_for_me'` and `PandaGallery.verdict?: PandaVerdict` (ticket 03). Optional, so existing fixtures stay valid.
  - `savePandaVerdict(galleryID: number, verdict: PandaVerdict | null): Promise<void>` (ticket 03): a verdict sends `PUT /api/collector/catalog/verdicts/{id}` with `{"verdict": ...}` and `Content-Type: application/json`; `null` sends `DELETE` to the same path. Both use `keepalive: true` and `collectorMessages`, and take no abort signal: leaving the page must not cancel a save.
- `web/src/galleries/GalleryBox.tsx` (ticket 03): an optional `actions?: ReactNode`. When it is given, `GalleryBox` wraps today's output in one positioned element (`gallery-box-cell`, filling the cell's height like `.gallery-box`) and renders `actions` in a sibling of the link (`gallery-box-actions`), after the link in DOM order and never inside it. When it is not given, the output is exactly today's. CSS in `Galleries.css`:
  - The actions strip is absolutely positioned over the top edge of the cover slot, with a small inset. The cover's offset in the card is fixed: the card's padding (`p="3"`), the two-line title (`.gallery-box h2`) and the gap (`gap="2"`) are all set by `GalleryBox` and `Galleries.css`, so the offset is a `calc()` of the same tokens. Only the controls in the strip take pointer events, so the cover between them still opens the gallery.
  - Under `@media (hover: hover) and (pointer: fine)` (the pattern in `Reader.css`), a control that is not `aria-pressed="true"` is hidden unless the cell is hovered or contains a `:focus-visible` element. Hide with `opacity`, not with `display` or `visibility`: a hidden control must stay in the Tab order, or the keyboard never reaches it.
  - On coarse pointers each control is at least 44px wide as well as high.
  - The card's hover background applies while the pointer is anywhere in the cell, on a control too.
- `web/src/panda/PandaCard.tsx` (ticket 03):
  - `PandaCard({ gallery, onVerdict? })` with `onVerdict?: (gallery: PandaGallery, verdict: PandaVerdict | null) => Promise<void>`. The promise settles when the save settles and never rejects. Without `onVerdict` the card renders no controls and passes no `actions`, so the sizing card and `PandaCard.test.tsx` stay as they are.
  - With `onVerdict`, it passes two Radix Themes `IconButton`s of size `1` as `actions`: "Not for me" (`Cross2Icon`) and "Interesting" (`BookmarkIcon`, `BookmarkFilledIcon` when given). Not a star or a heart: those read as Panda favorites. Each has the verdict as its accessible name and `title`, and `aria-pressed`. The given one is `variant="solid"`, red for "Not for me" and the accent colour for "Interesting". A control that is not given is `variant="soft"` in gray on an opaque panel background (`var(--color-panel-solid)`), so it stays readable on any cover. These are starting values for the manual check.
  - A click calls `onVerdict` with the control's verdict, or `null` when that verdict is the given one, and marks the card pending until the promise settles. While pending, both controls have `aria-disabled="true"` and ignore clicks, and the clicked one shows a `Spinner` in place of its icon. Use `aria-disabled`, not `disabled` and not the `loading` prop (which sets `disabled`): a disabled button drops the keyboard focus. The card never shows a verdict from its own state; `aria-pressed` comes only from `gallery.verdict`.
  - Ticket 05 adds an optional `subdued?: boolean` that defaults to `!!gallery.displayed`, today's value. `PandaCatalog` passes `false` in a verdict view.
- `web/src/panda/PandaCatalog.tsx`:
  - Ticket 01: props and the listing object carry `view`. Everything that reads `unseen` today reads `view === 'unseen'`.
  - Ticket 03: every listed card gets `onVerdict`. It calls `savePandaVerdict`. On success it replaces that gallery in the loaded result by gallery ID: the new `verdict` (none after a removal), and `displayed: true` after a set. It also removes a failure message that names the same gallery. On failure it stores the gallery ID, the gallery title and the error message for the message of behaviour 33. It never bumps `attempt`, never changes the hash, and never sends a catalog or display request. A save that settles after a page change, or after another save settled, changes only its own gallery on the result that is current then, and nothing when that gallery is not on it.
  - Ticket 03: the failure message is a Radix Themes `Callout` (`size="1"`, `color="red"`, `role="alert"`) with the sentence and the error, the gallery title on a line of its own that is cut with an ellipsis, and an `IconButton` named "Dismiss". It is rendered in `.gallery-pagination`, which is already `position: relative`, in a wrapper that is absolutely positioned directly above that row and centred in it, at most as wide as the row. It overlays the bottom edge of the card area, above the cards and their controls, and leaves Previous and Next free. The wrapper has an opaque background (`var(--color-panel-solid)`) and a shadow, as `.gallery-tag-suggestions` has, because a callout's own background is translucent. The styles go in `PandaCatalog.css`. Being out of the flow, the message changes neither the height of `.gallery-viewport` nor the measured page capacity. It opens no dialog and never moves the focus, and the key handler needs no change for it.
  - Ticket 05: the view control has four items in the order "All", "Unseen", "Interesting", "Not for me", and sets the hash to the listing with the chosen `view` and no cursor. The heading and the control may wrap at phone width; nothing in the toolbar may overflow. The empty states of behaviours 27 and 28 show in a verdict view without a cursor; with a cursor the existing states of the view of all galleries show. "Catch up…" stays tied to the unseen view. Cards get `subdued={false}` in a verdict view.
- `web/src/App.tsx` passes `view` (ticket 01).
- Test seams: a new `web/src/panda/PandaVerdicts.test.tsx` for tickets 03 and 05, which renders `<App />` with its own stubbed `fetch` as `PandaCatalog.test.tsx` does (copy the `useGalleryGrid` mock). The existing test files stay unedited. jsdom applies no stylesheet, so the hover reveal, the touch sizes, the position on the cover and the position of the failure message are manual checks.

**Risky choices.**

- **No reload after a save.** The loaded page is patched in place. This is what keeps the card in the unseen view (behaviour 15), and it means a listing can show a state that a new load would not (behaviour 30).
- **A verdict writes a display.** `SetVerdict` adds a row to `panda_gallery_displays`, so "a gallery with a verdict is seen" holds in the existing unseen condition without a second probe, and a removed verdict leaves the gallery displayed without extra state.
- **The verdict page query reads all verdicts of one kind per page.** It joins and sorts every gallery with that verdict that passes the filters, then cuts the page. That is cheap while verdicts are few among millions, which the brief states. The manual check on a real database guards the plan.
- **A failure message that overlays the page.** It is the catalog's only alert outside the page flow. It never changes the page capacity and never interrupts paging, at the price of covering the bottom edge of the last row of cards until the user dismisses it. A failed save should be rare.

## Assumptions

- **A verdict view is a fourth value of the view control**, not a separate filter control. Rejected: a separate verdict menu beside "All/Unseen" that is disabled in the unseen view. One control can't express the forbidden combination.
- **Link parameter `verdict=interesting|not_for_me`**, and a link with both `verdict` and `unseen=true` opens the verdict view. Rejected: an error page for the combination, and `view=` as a new parameter that would change existing unseen links.
- **A click on the given verdict's control removes it.** Rejected: a third "remove" control or a menu.
- **A verdict stores a display at the time of the verdict.** Rejected: treating "has a verdict" as a second source of displayed state in each query. A removed verdict would then need its own marker to stay displayed.
- **One row per gallery, no history.** Removing a verdict deletes the row, and changing it replaces verdict and time. Rejected: an append-only log of verdict changes. The brief asks for identity, verdict and time.
- **Giving the same verdict again keeps its time.** Rejected: refreshing the time on every save.
- **A verdict needs no catalog row**: every positive gallery ID is accepted, as for display records. Rejected: 404 for a gallery without a catalog row, which would fail during a removal and re-projection.
- **`PUT`/`DELETE /api/collector/catalog/verdicts/{id}`.** Rejected: one `POST` with an empty verdict meaning removal.
- **Paging keys keep working with the focus on a verdict control** (behaviour 12). Rejected: ignoring them on buttons, which would break "click verdicts, press `d`".
- **No optimistic display.** The card shows a spinner, then the stored verdict. Rejected: showing the verdict at once and reverting on failure, which would show an unsaved verdict. The spinner shows at once. Rejected: a spinner that appears only after a delay, which picks a timing constant before anyone has felt the latency against a real collector (see **Manual checks**).
- **The failure message is a non-modal alert outside the page flow**, over the bottom edge of the card area, and it shows the last failure only. It stays until the user dismisses it, a later failure replaces it, or a later save for the same gallery succeeds. Rejected: an in-flow alert like the record-failure alert, which changes the page capacity, loads the page again, and removes judged cards from the unseen view. Rejected: a modal dialog, which can't be missed but traps the focus, so a failure that settles while the user pages with the keys opens on a later page and swallows the next `d`. Rejected: a message that goes away by itself after some seconds, which adds a timing constant and can be missed.
- **The details tooltip keeps Radix's default behaviour**, so it can stay open while the pointer is on a control, without covering it. Rejected: `disableHoverableContent` on the `Tooltip` of a card with `actions`. It closes the tooltip as soon as the pointer leaves the link, but it also stops the pointer from moving onto the tooltip of a Panda card, which works today, for something the brief doesn't ask.
- **Cards in a verdict view are not subdued** (behaviour 26). Rejected: the subdued look everywhere a gallery is displayed, which would dim every card of these views for no information.
- **Next in a verdict view records the page**, like every listing. Rejected: a special case that skips the record there.
- **An expunged gallery with a verdict needs "Include expunged" to be listed** in a verdict view, as in every listing.
- **A listing shows the verdicts of its last load** (behaviour 30). A load that was already under way when a save settled can show that gallery without its verdict until the next load. Rejected: restarting such a load, or a client-side overlay of saved verdicts, for a window of a few milliseconds.
- **A resize that changes the page size counts as leaving the page** in the unseen view (behaviour 15), because the listing loads again. Rejected: keeping judged cards across loads with a client-side list.
- **Empty-state wording** (behaviours 27–29) and the **failure message wording** (behaviour 33) are mine.
- **Icons and colours**: a cross in red for "Not for me", a bookmark in the accent colour for "Interesting".
- **Docs**: `CONTEXT.md` gains **Panda gallery verdict** as the brief words it. The entry **Displayed Panda gallery** is extended to cover a gallery the user gave a verdict, because the brief makes such a gallery displayed. ADR 0019 gains a paragraph on verdicts instead of a new ADR. "Verdict view" and "neutral" are not added as terms: the spec defines them where it uses them.
- **Tana and the collector are updated together.**

## Out of scope

- A verdict for a full page or a selection, a reason or note with a verdict, and verdict controls on the Panda lookup page or the local gallery listing.
- A selected card and keys that give a verdict.
- Hiding "Not for me" galleries from any listing, and counts or badges per verdict.
- Any use of Panda favorites, and any sync of verdicts to Panda.
- A history of verdict changes, undo, export, and the later evaluation of verdicts against metadata.
- A live update of other tabs or devices after a save.
- Compatibility between a new Tana and an older collector.

## Manual checks

- **Control look.** On covers that are light, dark and missing: the unpressed controls are readable, the pressed "Not for me" is clearly visible on a subdued card in the view of all galleries, and the pressed "Interesting" is distinct from it. Tune: icon choice, colours and variants, the `IconButton` size, the inset from the cover's corners.
- **Reveal.** With a mouse, controls show on hover and hide when the pointer leaves, except a given verdict. Tab from the card's link reaches "Not for me", then "Interesting", and both show while the focus is in the card. After a mouse click on a control, the unpressed control hides once the pointer leaves.
- **Touch.** On a phone or with a coarse pointer the controls always show, are at least 44px in both directions, and leave the status badge and the page count free. At the narrowest grid (two cards across) they don't touch each other.
- **Click targets.** A click on a control never opens Panda. A click on the cover between the controls and anywhere else on the card still does.
- **Tooltip.** On cards in the first and the last row, the details tooltip never covers the controls of its card. It may stay open while the pointer is on a control. Tune, only if a lingering tooltip gets in the way: `disableHoverableContent` on the `Tooltip` in `GalleryBox` when `actions` is given.
- **Card size.** The number of galleries per page and the card height are the same as before the feature at the same window size, and the local gallery listing looks and behaves as before.
- **Unseen view.** Giving verdicts to several cards on a page leaves every card in place. After "Reload results" they are gone. Resizing the window so the page size changes also removes them (accepted, see **Assumptions**).
- **Save latency.** Against a real collector reached over the network, not one on the same machine, give verdicts to 5 to 10 cards in a row, some of them before the earlier saves came back. Good: the pressed state arrives fast enough to feel direct, the spinner doesn't flash distractingly on fast saves, and a card that is still saving doesn't hold up the next card. Tune: show the spinner only after a short delay (a new timing constant, to list here once picked), a calmer pending look than a spinner, and the `IconButton` size (`1`).
- **Failure.** With the collector stopped, a click shows the spinner, then the message over the bottom edge of the card area, and the card is as before. When the message appears and when it is dismissed, the number of cards and their positions stay the same and the listing doesn't load again: in the unseen view, cards judged earlier stay. The message covers no verdict control and neither Previous nor Next, also on a phone, and a long gallery title doesn't make it tall. Give a verdict and press `d` at once with the collector stopped: the message shows on the next page, the focus stays in the page, and `d` keeps paging. A verdict view opened with the collector stopped shows the catalog's usual error display. Tune: the message's position, width and title cut.
- **Verdict views.** Cards are not subdued. The toolbar with the four-item view control doesn't overflow at 360px width. The empty states read well.
- **Query plan.** On a real collector database, `EXPLAIN (ANALYZE)` of a verdict view page, with and without a search and a category filter, starts from `panda_gallery_verdicts` and reaches `catalog_galleries` by primary key. No ordered scan of the catalog.
- **Shared state.** A verdict given on one device shows on another after a reload, and after restarts of Tana and the collector.
- **Guards the structure gives.** No test pins these. In review: `CatchUp`, `RecordDisplays` and the projection never write `panda_gallery_verdicts`, and the table has the three columns of the migration and no foreign key. By hand: after a catch-up, the verdicts given before are still on their cards and no other card has one, and the newer version of a gallery with a verdict shows none.
- **Absences.** No control gives a verdict to a full page, nothing asks for a reason, no key gives a verdict, no card is selected, the Panda lookup page has no verdict controls, and a gallery that is a Panda favorite shows no verdict until the user gives one.
- The plan picks no timing constant. The spinner shows at once.
