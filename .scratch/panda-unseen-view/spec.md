# Unseen view for the Panda catalog

## Problem

The Panda catalog only lists galleries newest first. Tana doesn't know which galleries the user has already looked at, so each visit starts at the top and the user has to page down until the galleries look familiar. A backlog of old galleries never goes away, and with a search or a category filter there is no way to tell new matches from ones already passed.

## Solution

A view control in the Panda toolbar switches to the unseen view, which lists only galleries the user hasn't seen. Going to the next page in any listing marks the page the user leaves as seen. A catch-up action in the unseen view marks the current page and everything older as seen in one step, after a confirmation. In the view of all galleries and in searches, galleries already displayed get a subdued look. The seen state lives on the collector, so every device and browser tab sees the same state.

## Fit

Fits, with one small web prefactor (ticket 01). Parts of the design that exist only because of how the code is built today:

1. **The page query runs in the collector database (ADR 0018), and Tana keeps its own tables.** The unseen condition has to be part of `catalogPageQuery`, so the seen state lives in the collector database. Passing Tana's seen IDs to the collector would send a growing list on every page. *Not a workaround:* the collector stores the seen state and a new ADR 0019 records this growth of collector ownership beyond ADR 0004 (ticket 02).
2. **`catalogPageQuery` reads candidates in index order and stops at the limit.** *Fits:* the unseen condition is a correlated `NOT EXISTS` probe per candidate, the same shape as the `-tag` check in `tagMatcher.match`, and the catch-up coverage is one keyset bound `(ordered_at, gallery_id) > position`, read before the query is built, the way `tagMatcher` resolves tag IDs first.
3. **`List` recovers from an emptied page with a reversed, inclusive cursor, and returns previous cursors.** In the unseen view both would lead back to passed pages. *Kept:* `CatalogOptions` gains `Unseen bool`, like `IncludeExpunged`, and `List` applies the unseen paging rules under it (behaviours 15–17). The rules are behaviour of the view, not a workaround.
4. **`Project` and `ProjectFeed` replace catalog rows, and `catalog_gallery_tags` cascades from `catalog_galleries`.** *Fits:* the seen tables have no foreign key to catalog rows, and the catch-up stores position values, not a row reference.
5. **`useGalleryGrid` sets the page size from the viewport.** *Fits:* the browser records the IDs of the galleries it rendered, never a cursor range.
6. **`PandaCatalog.tsx` moves to the next page in two ways: the Next link's `href` and a key handler that sets the hash.** Recording would have to be added twice. *Prefactor (ticket 01):* one next-page function serves the Next link (plain clicks) and the ArrowRight/d shortcuts.
7. **`pandaListingLink` takes five positional arguments, three of them booleans, and `listPandaCatalog` takes seven.** A sixth flag on every call invites a swapped boolean. *Prefactor (ticket 01):* both take one `PandaListing` object.
8. **`PandaCatalog.test.tsx` answers every unknown `fetch` with a 200 catalog page.** *Kept:* the record request of ticket 03 goes to a path that this catch-all answers with 200, so the existing tests stay unedited. New tests install their own handlers.
9. **Tana and the collector are deployed independently.** An older collector ignores `unseen` and returns 404 for the new routes. *Kept:* the user updates both together (see **Out of scope**). A failed record only shows a message and never blocks paging (behaviour 11).

## Behaviour

Terms: *listing* is the Panda catalog page in Tana (`#/panda`) in any view or search. *Catalog position* is a place in the catalog order: `ordered_at` (Panda publication time, or first sighting while unknown), then gallery ID. *Newest gallery on the page* is the first one shown.

Unseen view:

1. The Panda toolbar has a view control with two options, "All" and "Unseen". `#/panda` without `unseen` shows all galleries. This is the start state, and the Panda link in the navigation opens it.
2. Choosing "Unseen" moves to the listing link with `unseen=true` and drops the cursor. Choosing "All" removes `unseen` and drops the cursor. Reloading or sharing a link keeps its view. No setting stores the view, so `#/panda` always opens "All".
3. The unseen view lists only galleries that are not seen. It combines with the search, the categories, the default filter and its bypass, and "Include expunged". Changing any of them keeps `unseen=true`, and the collector applies all of them together. "Clear all filters" keeps the current view.
4. The unseen view keeps the catalog order: catalog position, newest first.
5. The seen state lives on the collector. Every device and browser tab sees the same state, and the state survives restarts of Tana and the collector.
6. The listing shows no count of unseen galleries, and the navigation shows no badge.

How a gallery becomes seen:

7. In every listing (the view of all galleries, each search, and the unseen view), going to the next page records the galleries that the current page displayed as displayed Panda galleries. Each display keeps the time of its record.
8. Going to the next page means a plain click on Next (or Enter on it), or the next-page shortcuts ArrowRight and `d`. A click on Next with Ctrl, Meta or Shift held, or with the middle button, is left to the browser and records nothing.
9. Nothing else records: not a reload, not "Reload results", not a resize, not Previous or its shortcuts (ArrowLeft, `a`), not a new search, not a category, default-filter or "Include expunged" change, not the view control, not a click on a card, and not a catch-up.
10. A record covers exactly the galleries rendered on the page the user leaves, whatever the page size is after a resize. Galleries that a search or filter hides are not on the page and stay unseen. A page with no galleries sends no record.
11. Next moves to the next page at once. The record goes out in the background and survives a reload right after the click.
12. A gallery displayed while it had provisional Panda metadata stays displayed after collected metadata replaces it, and after its catalog row is removed and projected again.
13. The seen state belongs to a gallery ID. A new version of a seen gallery has its own gallery ID and is unseen until it is displayed or covered itself.
14. There is no action on a single gallery: no "mark as seen" and no "mark as unseen".

Paging in the unseen view:

15. Every non-empty page of the unseen view has an active Next, including the last page. Next on the last page records it and shows the empty state of behaviour 30.
16. The unseen view has no way back: Previous stays disabled and its shortcuts do nothing.
17. An unseen-view link with a cursor shows the unseen galleries after that position. When the browser's Back control returns to an earlier link whose galleries are now seen, the listing shows the unseen galleries after that position, or the empty state of behaviour 30. Never an error, and never a page without a way on.
18. Galleries that join the catalog during a visit are unseen. Ones newer than the cursor do not shift the current or following pages. They appear when the user returns to the newest unseen galleries (the unseen view without a cursor).

Catch-up:

19. In the unseen view, the toolbar has a "Catch up…" button. It is disabled while the listing loads, while it shows stale results after an error, and when the page shows no galleries. The view of all galleries has no such button.
20. The button opens a confirmation titled "Catch up?". It reads: "This page and every older Panda gallery, back from <date>, become seen, including galleries that the current search and filters hide. Newer galleries keep their state. This can't be undone." The date is the newest gallery's `posted_at`, or its `first_seen_at` while that is unknown, in local time. "Cancel" closes it and changes nothing.
21. "Catch up" makes all galleries at and before the catalog position of the newest gallery on the page seen, across the full Panda catalog. The listing then shows the newest unseen galleries with the same search and filters.
22. A catch-up never makes a gallery unseen. A catch-up at a position older than an earlier one changes nothing. Galleries newer than the position keep their state.
23. A gallery that joins the catalog later with a catalog position at or before the catch-up position stays out of the unseen view.
24. A catch-up creates no display records.
25. While the catch-up request is pending, "Cancel" and "Catch up" are disabled and the confirmation can't be closed, by Escape either. If the catch-up fails, the confirmation stays open and shows an alert inside it: "Catch-up failed. Nothing was marked as seen." followed by the error. The page behind it stays as it was. "Catch up" tries again, and "Cancel" closes the confirmation. The alert goes away when the next try starts, and the confirmation never opens with an alert from an earlier try. When the newest gallery has left the catalog, the error reads "This gallery is no longer in the Panda catalog. Reload and try again."
26. Arrow keys and `a`/`d` pressed while the confirmation is open don't change the page.

Seen cards outside the unseen view:

27. In the view of all galleries and in searches, the card of a displayed gallery has a subdued look and gives assistive technology the word "Seen".
28. A gallery that only a catch-up covers keeps the normal look.

Empty states and failures:

29. An empty unseen view without a cursor reads "No unseen Panda galleries." with "You have seen every Panda gallery. New galleries appear here once feeds are collected.", and offers "Show all galleries". With a search, categories or an applied default filter, it reads "No unseen Panda galleries match." with "Try different titles, tags, or categories, or show all galleries.", and offers "Show all galleries".
30. An empty unseen view with a cursor reads "No more unseen Panda galleries." ("No more unseen Panda galleries match." with a search or filter) with "Galleries that arrived during this visit wait with the newest unseen galleries.", and offers "Newest unseen galleries" and "Show all galleries".
31. "Show all galleries" opens the view of all galleries with the same search and filters and no cursor. "Newest unseen galleries" opens the unseen view with the same search and filters and no cursor. These empty states never read "No Panda galleries yet.", which only the view of all galleries shows.
32. If a record fails, the galleries of that page stay unseen, and an alert reads "Could not mark a page as seen. Its galleries stay unseen." followed by the error. It shows between the toolbar and the galleries, below the stale-results alert when both show, and has no Retry, because the page is already passed. Each record that settles sets or clears the alert, in the order the records settle: a failure shows it, a success removes it. So a failure that settles after the record of a later page succeeded still shows. The alert is dropped when the user leaves the Panda catalog for another Tana page or the reader. A record that settles after that still stores or fails on the collector, but shows nothing. A failed record marks nothing. A record never covers galleries that the user did not pass.
33. If the collector is unreachable, the unseen view shows the current error display of the Panda catalog: the stale-results alert, or "Panda catalog unavailable." on a fresh visit.

Guards:

34. Display records and the catch-up coverage are separate facts in the collector database. Each display keeps its gallery ID and time. The catch-up keeps its position and the time of the catch-up that set it.
35. The seen state survives every change of the catalog projection: provisional-to-collected replacement, tag replacement, and removal and re-projection of a catalog row.

Local API:

36. `GET /api/collector/catalog` accepts `unseen` (a boolean; anything else gives 400 `invalid_query`) and forwards it to the collector. Each item carries `displayed`.
37. `POST /api/collector/catalog/displays` with `{"gallery_ids": [...]}` (1 to 100 IDs, each positive) returns 204. Any other list gives 400 `invalid_batch` without calling the collector, and a body with unknown fields gives 400 `invalid_json`. 502 `collector_unauthorized` when the collector rejects Tana's token, 502 `collector_unavailable` when it can't be reached or fails, 503 `collector_not_configured` without a collector.
38. `POST /api/collector/catalog/catch-up` with `{"gallery_id": n}` returns 204. `n <= 0` gives 400 `invalid_gallery_reference` without calling the collector. A gallery without a catalog row gives 404 `gallery_not_found`. The other errors are as in behaviour 37.

## Design

**Data flow.** The browser talks only to the local service. Listing: `GET /api/collector/catalog?...&unseen=true` → `collectorapi.Client.Catalog` → collector `GET /api/catalog?...&unseen=true` → `catalog.Service.List`. Record: browser `POST /api/collector/catalog/displays` → `collectorapi.Client.RecordCatalogDisplays` → collector `POST /api/catalog/displays` → `catalog.Service.RecordDisplays`. Catch-up: browser `POST /api/collector/catalog/catch-up` → `collectorapi.Client.CatalogCatchUp` → collector `POST /api/catalog/catch-up` → `catalog.Service.CatchUp`. Non-test code in `internal/local/` reaches the collector only through `internal/collectorapi`.

### Collector schema (`internal/collector/storage/migrations`)

- `0002_panda_gallery_displays.sql` (ticket 02):
  ```sql
  CREATE TABLE panda_gallery_displays (
      gallery_id BIGINT NOT NULL CHECK (gallery_id > 0),
      displayed_at TIMESTAMPTZ NOT NULL,
      PRIMARY KEY (gallery_id, displayed_at)
  );
  ```
  A comment states why it has no foreign key: the seen state is the user's history, not a projection, and must survive every change of `catalog_galleries` (ADR 0019). The primary key leads with `gallery_id`, so the unseen probe and the `displayed` flag are index lookups.
- `0003_panda_catalog_catch_up.sql` (ticket 06): a singleton `panda_catalog_catch_up (id INTEGER PRIMARY KEY CHECK (id = 1), ordered_at TIMESTAMPTZ NOT NULL, gallery_id BIGINT NOT NULL, caught_up_at TIMESTAMPTZ NOT NULL)`. No row means no catch-up. No foreign key.
- Migrations must be numbered in sequence (`pgdb.Open`); ticket 06 comes after ticket 02. No sqlc query changes, so `go generate` is not needed.

### Collector: `internal/collector/catalog`

- `var ErrInvalidDisplays = errors.New("invalid catalog displays")`
- `func (s *Service) RecordDisplays(ctx context.Context, ids []int64) error`: 1 to `collectorapi.MaxCatalogDisplays` IDs, each `> 0`, or `ErrInvalidDisplays` and nothing stored. A record stores the whole page or nothing. All displays of one record carry the same time: the collector database's time of the record. Duplicate IDs in one record give one display and don't fail the record. It records every ID it is given, known to the catalog or not.
- `List` (ticket 02) sets `CatalogItem.Displayed` for every item in every view: true when the gallery has at least one display. It looks up only the page's items by gallery ID, not every candidate, and the page query itself doesn't change.
- `List` with `options.Unseen` (ticket 04):
  - A cursor with `before` returns `ErrInvalidPagination`. The UI never creates one in the unseen view.
  - The unseen condition (the gallery has no display) goes into the shared predicate, so each multi-category `UNION ALL` branch carries it. **It must stay a correlated probe per candidate, like the `-tag` check in `tagMatcher.match`. Never read the display table into an array or a `NOT IN` list.**
  - `NextCursor` is `itemCursor(last item)` whenever the page has items, with or without `more`. `PreviousCursor` is always empty. An empty page returns neither cursor (no reversed recovery).
- `var ErrGalleryNotInCatalog = errors.New("gallery not in catalog")`
- `func (s *Service) CatchUp(ctx context.Context, galleryID int64) error` (ticket 06): `galleryID <= 0` returns `ErrInvalidGalleryReference`. A gallery without a `catalog_galleries` row returns `ErrGalleryNotInCatalog`. Otherwise the stored position becomes that row's current `(ordered_at, gallery_id)`, with the time of this catch-up, only when it is greater than the stored position. Catch-ups that run at the same time only ever advance the position: the stored position is the greatest one requested, and its time is that of the catch-up that set it. ADR 0018 keeps the same kind of guarantee for metadata eligibility.
- `List` with `options.Unseen` (ticket 06) reads the catch-up position in its transaction before building the query. When one exists, it limits candidates to positions after it, as a keyset bound on `(ordered_at, gallery_id)` with bound arguments, so the ordered index scan stops at the position.
- Test seam: package tests on `pgtest`, using `openCatalog`, `retain`, `projectFeed` and `assertCatalogIDs` from the existing test files, in a new `seen_test.go`. Tests may read `panda_gallery_displays` directly for display times.

### Collector: `internal/collector/httpapi`

- `HandleCatalog` parses `unseen` like `include_expunged` (`strconv.ParseBool`; an error gives 400 `invalid_query`).
- `func HandleRecordCatalogDisplays(service *catalog.Service) http.Handler`, registered as `POST /api/catalog/displays`. Body `{"gallery_ids": [...]}` via `server.DecodeJSON`. 204 with no body on success.
- `func HandleCatalogCatchUp(service *catalog.Service) http.Handler`, registered as `POST /api/catalog/catch-up`. Body `{"gallery_id": n}`. 204 on success.
- `catalogError` gains `ErrInvalidDisplays` → 400 `invalid_batch` and `ErrGalleryNotInCatalog` → 404 `gallery_not_found`. `ErrInvalidGalleryReference` already maps to 400 `invalid_gallery_reference`.
- Test seam: the table in `catalog_test.go` for query and body validation. The success paths run end to end through the local httpapi tests.

### `internal/collectorapi`

- `const MaxCatalogDisplays = 100`
- `CatalogOptions.Unseen bool`, sent by `Catalog` as `unseen=<bool>` on every request, like `include_expunged`.
- `CatalogItem.Displayed bool` with JSON `displayed`.
- `func (c *Client) RecordCatalogDisplays(ctx context.Context, ids []int64) error` posts `{"gallery_ids": ids}`. 204 returns nil. Any other status returns `downloadResponseError(response)` (`*HTTPError` with `Code`).
- `func (c *Client) CatalogCatchUp(ctx context.Context, galleryID int64) error` posts `{"gallery_id": galleryID}`, with the same result handling.

### Local: `internal/local/httpapi`

- `HandleCollectorCatalog` parses `unseen` like `include_expunged` and sets `options.Unseen`.
- `func HandleCollectorRecordCatalogDisplays(client *collectorapi.Client) http.Handler`, registered as `POST /api/collector/catalog/displays`. It validates the list itself (behaviour 37) before calling the client. 204 with no body on success.
- `func HandleCollectorCatalogCatchUp(client *collectorapi.Client) http.Handler`, registered as `POST /api/collector/catalog/catch-up`. It validates `gallery_id > 0` itself. 204 on success.
- Both use `catalogCollectorConfigured` and `writeCatalogCollectorError`. `writeCatalogCollectorError` gains upstream 404 with code `gallery_not_found` → 404 `gallery_not_found`.
- New handlers go in a new file `catalog_seen.go`, with routes next to the catalog routes in `routes.go`.
- Test seam: `catalog_test.go`, running the real collector handler behind `httptest.NewServer` (`collectorhttp.NewHandler(&collector.App{Logger, Catalog: catalog.New(db, origin), APIToken})`), as `TestLocalCollectorSyncDownloadCategories` does. Seed catalog rows with `gallery_refs` inserts and `catalog.Project` in a transaction.

### Web

- `web/src/navigation.ts` (ticket 01):
  - `export interface PandaListing { search: string, cursor: string, includeExpunged: boolean, categories: string[], bypassDefault: boolean }`. Ticket 05 adds `unseen: boolean`.
  - `export function pandaListingLink(listing: Partial<PandaListing>): string` writes the parameters in today's order: `q`, `cursor`, `include_expunged`, each `category`, `bypass_default`, each only when set. Ticket 05 appends `unseen=true` last, only when set.
  - The panda route is `{ kind: 'panda' } & PandaListing`. Ticket 05 parses `unseen` as `params.get('unseen') === 'true'`.
- `web/src/panda/api.ts`:
  - `listPandaCatalog(listing: PandaListing, pageSize: number, signal?: AbortSignal)` (ticket 01) builds today's query string. Ticket 05 appends `unseen=true` last, only in the unseen view, so existing URL assertions hold.
  - `PandaGallery.displayed?: boolean` (ticket 03). It's optional, so existing fixtures stay valid.
  - `recordPandaDisplays(galleryIDs: number[]): Promise<void>` (ticket 03) posts `{"gallery_ids": [...]}` with `Content-Type: application/json` (as `savePandaDefaultFilter` does; `server.DecodeJSON` requires it) to `/api/collector/catalog/displays` with `keepalive: true` and `collectorMessages`. It takes no abort signal: leaving the page must not cancel a record.
  - `catchUpPanda(galleryID: number, signal: AbortSignal): Promise<void>` (ticket 07) posts `{"gallery_id": n}` to `/api/collector/catalog/catch-up` with `collectorMessages` plus `gallery_not_found: 'This gallery is no longer in the Panda catalog. Reload and try again.'`.
- `web/src/panda/PandaCatalog.tsx`:
  - Props are `PandaListing` (ticket 01). `App.tsx` passes the route's listing fields. Effects keep depending on the individual fields (`useRoute` memoizes `categories` by value). A listing object built during render must not become an effect dependency, or every render refetches.
  - One next-page function (ticket 01) serves the Next link (`onClick` on a plain primary click without modifiers: `preventDefault`, then navigate; other clicks keep the native link) and the ArrowRight/`d` shortcuts. Ticket 03 makes it start `recordPandaDisplays(result.items ids in page order)` when the page has items, then navigate without waiting. Next is only active when the result is current (not loading, no error), so the IDs are the rendered galleries.
  - Record-failure alert (ticket 03, behaviour 32): state in `PandaCatalog`. Each record, when it settles, sets the alert to its error (rejected) or clears it (resolved), in settle order, with no tracking of which record started last. `App.tsx` renders `PandaCatalog` without a key, so the state lives across listing changes within `#/panda` and goes when the component unmounts. A record that settles after unmount needs no guard, because React 19 drops that state update silently. The alert is a `Callout` with `role="alert"` and no button, placed directly after the stale-results `Callout`, above `.gallery-viewport`. Its height changes the measured page capacity, so the current cursor reloads once at the new page size when the alert appears and once when it goes, as with the stale-results alert. Records stay exact through that reload: Next is disabled while loading and records the items then rendered.
  - View control (ticket 05): a Radix Themes `SegmentedControl` with items "All" and "Unseen" in the toolbar. Changing it sets the hash to the listing with the new `unseen` and no cursor.
  - Empty states of the unseen view (ticket 05): behaviours 29–31. The view of all galleries keeps its three states unchanged.
  - Catch-up (ticket 07): a "Catch up…" button in the toolbar in the unseen view, and an `AlertDialog` built like the delete confirmation in `collector/Downloads.tsx`: the text of behaviour 20, the failure alert as `role="alert"` text inside the dialog, and both buttons disabled while the request is pending. `useMutation` from `../collector/useMutation` runs the request. The dialog opens with no error, and a new try clears the old one (`run` already does). While `pending`, the dialog ignores requests to close (Escape). The dialog's "Catch up" is also disabled whenever the toolbar's "Catch up…" would be, so a hash change behind the open dialog never sends a gallery from a stale page. On failure the dialog stays open. On success it closes the dialog, sets the hash to the listing with no cursor, and bumps `attempt` when the hash is already cursor-less, so the listing reloads. The key handler's ignore selector gains `[role="alertdialog"]`. The failure alert lives in the dialog, so it never changes the page capacity.
- `web/src/galleries/GalleryBox.tsx` (ticket 03) gains an optional `subdued?: boolean`. When it is set, GalleryBox adds the class `gallery-box-subdued` to the anchor and renders `<VisuallyHidden>Seen</VisuallyHidden>` inside the anchor, right after the title heading. The link's text then includes "Seen", and the card keeps its height (`VisuallyHidden` is out of flow). `PandaCard` only passes `subdued={!!gallery.displayed}`. `status` keeps its meaning: feed-metadata badges on the cover. The local Galleries page doesn't set `subdued` and renders unchanged. CSS in `Galleries.css`: `.gallery-box-subdued .gallery-cover, .gallery-box-subdued h2 { opacity: 0.5; }`.
- Test seams: `PandaCatalog.test.tsx` (renders `<App />` with a stubbed `fetch`; new tests install their own `fetchMock` implementation and keep the existing ones untouched), and `PandaCard.test.tsx` for the subdued card.

**Risky choices.**

- The collector stores user history (ADR 0019). It is the only place where the page query can test seen state per candidate.
- The record is fire-and-forget. Paging never waits for the collector's write, and a failure only shows a message. The cursor already puts the left page behind the next one, so the next page never depends on the record having landed.
- Catch-up is a single high-water position. It covers galleries that join later before it (behaviour 23) without one row per gallery, and it cannot move back.

## Assumptions

- **The collector stores the seen state, recorded in a new ADR 0019.** Rejected: Tana tables with the seen IDs passed to the collector, which lists the seen set on every page, and filtering in Tana after the collector's page, which breaks the page fill.
- **One display row per gallery and record, keyed `(gallery_id, displayed_at)`.** Rejected: first/last display columns, which lose the time of each display that the later verdict feature needs. Also rejected: storing the view or query of each display, which the brief doesn't ask for.
- **The display time is the collector's transaction time.** Rejected: a browser-supplied time, which depends on the device clock.
- **The seen tables have no foreign keys.** Rejected: a foreign key to `catalog_galleries` (loses state when a catalog row goes away) or to `gallery_refs` (couples the user's history to inventory changes). The record accepts any positive ID; only the local service calls it, with IDs from a catalog page.
- **The catch-up is one position that only advances, with the time of the catch-up that set it.** Rejected: one row per covered gallery (millions of rows, misses galleries that join later) and a log of every catch-up (only the newest position covers anything).
- **The catch-up position is the newest gallery on the current page, inclusive.** That covers the whole page without recording it as displayed (the brief allows no other recording event). Rejected: the oldest gallery on the page, which would leave the rest of the page unseen.
- **The browser sends the newest gallery's ID, and the collector reads its current catalog position.** Rejected: sending a cursor or a timestamp from the browser.
- **A record doesn't block paging.** Next navigates at once and a failure shows a message afterwards. Rejected: waiting for the record before navigating, which adds a collector round trip to every page turn and would block all paging if the record route fails.
- **Modified or middle clicks on Next record nothing.** The user stays on the page. Rejected: recording on every activation of the link.
- **The record-failure alert sits in the existing alert slot above the galleries, and the page reloads once when it appears or goes.** The stale-results alert already works this way. Rejected: an overlay on the grid, which would cover cards that Next then records as displayed. Also rejected: the pagination row, whose fixed 32px height is too small for the message at phone width.
- **Each settled record sets or clears the record-failure alert, in the order the records settle.** A failure that settles after a later page's success still shows. Rejected: following only the most recently started record, which hides a failure whose response arrives after the next page's record succeeded.
- **The record-failure alert lasts while the Panda catalog stays open.** Rejected: keeping it across Tana pages, which would need state outside `PandaCatalog` for a message about a page already passed.
- **A failed catch-up keeps its confirmation open and shows the error inside it, like deleting a download in `collector/Downloads.tsx`.** The confirmation can't be closed while the request is pending, for at most the collector client's existing 30-second timeout. Rejected: closing the confirmation and showing a page alert, which shrinks the grid and needs its own rule for when it clears.
- **GalleryBox renders the hidden "Seen" when `subdued` is set.** Rejected: overloading `status`, which renders badges on the cover, and a general children slot that no other card needs.
- **In the unseen view, the collector returns a next cursor on every non-empty page, never a previous cursor, no recovery cursor, and rejects backward cursors.** Rejected: special cases for Previous in the browser, which would leave old links able to reach passed pages.
- **The view control is a segmented control "All" / "Unseen" in the toolbar, and the parameter is `unseen=true`, added last only when on.** Rejected: an item in the Options menu, which hides the main control of the feature. "Clear all filters" keeps the view, because the view isn't a filter.
- **Empty-state, alert and dialog texts as written in behaviours 20, 25 and 29–32.**
- **The subdued look is 50% opacity on the cover and title, plus a visually hidden "Seen".** Rejected: a visible "Seen" badge, which adds a mark to most cards of a long browse.
- **The "Catch up…" button is in the toolbar, not in the pagination row.** It acts on the whole catalog, not on paging.
- **After a catch-up, the listing shows the newest unseen galleries.** Rejected: staying at the current cursor, which would always be empty.
- **A catch-up on a gallery that left the catalog returns 404 `gallery_not_found`.** Rejected: falling back to its last known position, which the collector doesn't keep.
- **Glossary.** The four terms from the brief go into `CONTEXT.md` as written. "Catalog position" (a place in the order of `ordered_at`, then gallery ID) and "listing" (the Panda catalog page in Tana) stay plain words in the spec and are not added as terms.

## Out of scope

- A count of unseen galleries and a navigation badge.
- Marking or unmarking a single gallery, and undoing a catch-up.
- Seen state for the local Galleries page.
- Verdicts or any other use of the display records. A later feature reads them.
- Recording the view or query of a display.
- Pruning old display records.
- Working against an older collector without the new routes. Tana and the collector are updated together.

## Manual checks

- **Query plan on a real collector database.** With millions of catalog rows, thousands of display records among the newest galleries, and a catch-up position set, run `EXPLAIN (ANALYZE, BUFFERS)` on the unseen page query (log it or rebuild it from `List`). Good: an index scan in catalog order with a nested-loop anti-join probing `panda_gallery_displays_pkey` per candidate, no hash or sort over `panda_gallery_displays`, and a time close to the same page in the view of all galleries. Repeat with two categories (the `UNION ALL` path) and with an excluded tag.
- **Subdued look.** In the view of all galleries, after paging forward and back, displayed cards are clearly quieter than unseen ones in light and dark mode, yet titles stay readable. Hover and focus outlines still show. Tune the opacity (`0.5`) in `Galleries.css`.
- **Toolbar layout.** At desktop width and at phone width (≤640px), the view control and "Catch up…" fit the toolbar without overflow, wrap cleanly with the search, categories and default-filter controls, and stay tappable.
- **Paging feel.** In the unseen view against a real collector, Next feels as fast as in the view of all galleries. After several pages, the browser's Back control shows remaining unseen galleries or the "No more unseen" state, never an error. A reload right after Next still records the page (check that the page is gone from the unseen view).
- **Resize and reload.** Resizing the window and reloading the page send no request to `/api/collector/catalog/displays` (network panel). After a resize changes the page size, Next records exactly the galleries now shown. While the listing loads, "Catch up…" is disabled.
- **Shared state.** Page forward in one browser, then open the unseen view in a second browser or device. The passed galleries are gone there too.
- **Catch-up dialog.** The text reads well, the date matches the newest card's tooltip, and Escape, Cancel and the keyboard work. Arrow keys inside the dialog don't page. Make the catch-up fail (stop the collector with the dialog open): while pending, both buttons are disabled and Escape doesn't close the dialog. Then the error shows inside the dialog, and the grid behind it doesn't reflow.
- **Record failure and reflow.** Make the record route fail (for example, stop the collector after a page loaded, or point Tana at a collector with a wrong token), then press Next. Good: the next page loads, the record-failure alert appears above the galleries (below the stale-results alert, if that shows too) with no Retry button, and the page reloads at most once at the smaller size. The galleries then on screen are exactly the IDs that the next record sends (network panel). After the collector recovers, the next successful record removes the alert and the page reloads once at full size. The failed page's galleries are still in the unseen view. Leaving for another Tana page and coming back shows no alert.
- **Absences.** No unseen count appears anywhere, the navigation shows no badge, cards have no "mark as seen" or "mark as unseen" action, and nothing offers to undo a catch-up.
