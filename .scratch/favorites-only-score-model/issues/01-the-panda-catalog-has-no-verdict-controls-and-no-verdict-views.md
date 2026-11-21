# 01: The Panda catalog has no verdict controls and no verdict views

What to build: the Panda listing loses everything a user could do with a verdict. A card is its link and nothing else. The view control offers "All" and "Unseen". A saved link with a `verdict` parameter opens the first page of the listing as if `verdict` and `cursor` were absent. Tana no longer passes verdict routes or the `verdict` parameter to the collector. The collector still stores and serves verdicts until ticket 09; nothing in the browser or in Tana reaches them any more.

This is spec behaviours 1 to 4 and 7 (Tana and web side), the web part of 6, and the listing part of 29. Layers, as fixed in spec **Design → Web (Listing)**, **Routes** and **`internal/collectorapi`**:

- `web/src/navigation.ts`, `web/src/panda/api.ts`, `web/src/panda/PandaCard.tsx`, `web/src/panda/PandaCatalog.tsx`, `web/src/panda/PandaCatalog.css`, `web/src/galleries/GalleryBox.tsx`, `web/src/galleries/Galleries.css`.
- `internal/local/httpapi`: the two verdict routes and `catalog_verdicts.go` go; `HandleCollectorCatalog` no longer reads `verdict`.
- `internal/collectorapi`: `Client.SetCatalogVerdict`, `Client.RemoveCatalogVerdict` and their request helper go. The verdict constants, `ValidVerdict`, `CatalogOptions.Verdict` and `CatalogItem.Verdict` stay until ticket 09, because the collector still uses them.

Tests of removed behaviour are deleted, not ported, and no new test pins that something is gone (spec **Design → Test seams**): `web/src/panda/PandaVerdicts.test.tsx`, `TestLocalCatalogVerdicts`, and the verdict cases in `PandaScoreFilter.test.tsx` and `PandaCard.test.tsx` go.

Test seams: Vitest with a mocked `fetch` (`PandaCatalog.test.tsx`, `PandaScoreFilter.test.tsx`, `PandaCard.test.tsx`, `navigation.test.tsx`); `internal/local/httpapi/catalog_test.go` through the real collector handler.

Blocked by: None

Status: resolved

- [x] A card of the listing renders its link, its cover, its status and its tags, and no button.
- [x] The view control of the listing has exactly the items "All" and "Unseen", with "Unseen" selected for `#/panda`.
- [x] `#/panda?verdict=not_for_me` opens the unseen view: the catalog request carries `unseen=true` and no `verdict`. `#/panda?verdict=interesting&unseen=false&score_filter=hidden&q=x` requests `q=x` and `score_filter=hidden` without `unseen=true` and without `verdict`, and shows "Hidden only" selected in the score filter control.
- [x] `#/panda?verdict=interesting&cursor=abc` requests the catalog without `cursor` and without `verdict`, so it shows the first page of the unseen view. `#/panda?cursor=abc` without `verdict` still sends `cursor=abc`.
- [x] No link that the page builds from such a route carries `verdict`: the search, next-page, view, score filter, bypass and clear links of a listing opened with `verdict=interesting` hold no `verdict` parameter.
- [x] On a page with the score filter, the hover details of a card name the rule in `score.rule` ("Never hidden: …", "Hidden by the override for …", "Hidden: the score is below the threshold.").
- [x] Going to the next page still records the page's galleries, and the paging keys still work with the focus on a card.
- [x] Through the real collector handler, Tana's `GET /api/collector/catalog?verdict=not_for_me` returns the same items as the request without the parameter, including galleries without a verdict.
- [x] Removal: `internal/local/httpapi/routes.go` registers no route under `/api/collector/catalog/verdicts`, and `internal/local/httpapi/catalog_verdicts.go`, `Client.SetCatalogVerdict` and `Client.RemoveCatalogVerdict` no longer exist.
- [x] Removal: `isVerdictView`, `PandaVerdict`, `verdictLabels`, `savePandaVerdict`, `VerdictHandler`, the `actions` prop of `GalleryBox`, and the CSS rules `.gallery-box-cell`, `.gallery-box-actions` and `.panda-verdict-error*` no longer exist. A search for "verdict", "Interesting" and "Not for me" in any letter case finds nothing in the non-test files under `internal/local` and in `web/src/navigation.ts`, `web/src/panda/api.ts`, `web/src/panda/PandaCard.tsx`, `web/src/panda/PandaCatalog.tsx`, `web/src/panda/PandaCatalog.css` and `web/src/galleries/`, except the one place in `web/src/navigation.ts` that reads the `verdict` parameter to drop the cursor. (The score page keeps its verdict texts until ticket 05, and local tests call `service.SetVerdict` until tickets 02 and 04.)
- [x] The `go` and `frontend` checks of the green check pass, and `gofmt -l .` prints nothing.

## Resolution

Tana's verdict routes, `httpapi/catalog_verdicts.go`, `Client.SetCatalogVerdict` and `Client.RemoveCatalogVerdict` are gone, and `HandleCollectorCatalog` no longer forwards `verdict` (`TestLocalCatalogIgnoresVerdictParameter` replaces `TestLocalCatalogVerdicts`). In the web, `PandaView` is `'all' | 'unseen'`, the route drops the cursor of a link with `verdict` and reads `score_filter` from every link, and the card, `GalleryBox`, catalog, API and CSS lost the verdict controls, views, save and alert. New tests cover old verdict links, the links built from them, the two view items, a card without buttons, and the paging keys with the focus on a card.
