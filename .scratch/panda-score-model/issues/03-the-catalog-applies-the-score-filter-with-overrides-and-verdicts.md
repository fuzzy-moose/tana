# 03: The catalog applies the score filter with overrides and verdicts

What to build: as soon as one score override exists, a catalog page from the collector leaves out the galleries that the score filter hides: galleries with a "Not for me" verdict and galleries that an "always hide" override matches, unless an "Interesting" verdict or a "never hide" override shows them. A request can ask for the filtered listing, for all galleries, or for only the hidden ones. The page says that the filter is in force, and each gallery says which override rule matches it. The rule for the score of an active model comes in ticket 06, and the control in the UI in ticket 10.

This is the server side of spec behaviours 27, 29, 30, 31 (rules 1 to 3), 32, 33, 36, 37, 38 and 42, and behaviour 54. Its layers are specified in spec **Design**:

- `internal/collectorapi`: `ScoreFilterAll`, `ScoreFilterHidden`, `ValidScoreFilter`, `CatalogOptions.ScoreFilter` (sent as `score_filter` on every request), `CatalogResult.ScoreFilter` with `HiddenUnseen`, and `CatalogItem.Score` with `Rule` and `Override` (the lines marked ticket 03). `ThresholdTenths`, `Value` and `Weights` come in ticket 06.
- `internal/collector/catalog`: **Design → The score filter in `List`**, without rule 4. `List` reads the overrides inside its transaction with ticket 02's reader. The constraints of that section hold: the condition is part of the predicate that every category branch gets, it is never part of a verdict view, and it tests each candidate by key. `HiddenUnseen` is set only on a request without a cursor. `CatalogItem.Score` is looked up for the page's items only.
- `internal/collector/httpapi`: `HandleCatalog` passes `score_filter` to the options unparsed.
- `internal/local/httpapi`: `HandleCollectorCatalog` forwards `score_filter` unparsed. The collector's 400 `invalid_query` already passes through.

While no override exists, the filter is not in force and `List` must behave exactly as today. The existing assertions stay unchanged and pin that.

Test seams: package tests in `internal/collector/catalog` (new test file) with `openCatalog`, `retain`, `projectFeed`, `setVerdict`, `unseenPage` and `assertCatalogIDs`. One case in the validation table of `internal/collector/httpapi/catalog_test.go`, and one local test through the real collector handler in `internal/local/httpapi/catalog_test.go`.

Blocked by: 02

Status: resolved

- [x] Without any override, `List` returns the same items for `ScoreFilter` `""` and `"all"` as before this ticket, also for a gallery with a "Not for me" verdict, with a nil `ScoreFilter` in the result and no `Score` on any item. `"hidden"` returns an empty page.
- [x] With an "always hide" override for a tag, `""` leaves out the galleries that carry the tag, `"hidden"` lists exactly the hidden galleries in catalog order, and `"all"` lists every gallery. The same holds for an uploader override and for a category override. A gallery with an empty uploader or category matches no override.
- [x] In `"all"` and `"hidden"`, an item that an override matches has `Score.Rule` `"always_hide"` or `"never_hide"` and `Score.Override` with the override's label. An item that no override matches has no `Score`. The result has a non-nil `ScoreFilter` in every view, the verdict views included.
- [x] With an override present, a gallery with a "Not for me" verdict is left out by `""` and listed by `"all"` and `"hidden"` with its `Verdict`. A gallery with an "Interesting" verdict that an "always hide" override matches is listed by `""`, not by `"hidden"`, and its item still has `Score.Rule` `"always_hide"`.
- [x] A gallery that a "never hide" override and an "always hide" override both match is listed by `""`, with `Score.Rule` `"never_hide"`.
- [x] With two categories selected and more hidden galleries than one page at the newest positions, `""` returns a full page of galleries that are not hidden, in catalog order, and following `NextCursor` lists every other gallery that is not hidden exactly once.
- [x] With a default category and an override, `""` lists only galleries of that category that are not hidden, and `"hidden"` only hidden galleries of that category: the default filter applies before the score filter in every state.
- [x] After `CatchUp` on the newest gallery, the unseen view is empty in the `"hidden"` state too: a catch-up covers hidden galleries.
- [x] In the unseen view, hidden galleries are absent. After the page is recorded with `RecordDisplays` and the override is removed, the unseen view lists the formerly hidden galleries.
- [x] An empty page of the unseen view in the `""` state, requested without a cursor, has `ScoreFilter.HiddenUnseen` true when an unseen gallery that passes the search and filters is hidden, and false when none is, for example under a search that no hidden gallery matches.
- [x] An empty page of the unseen view that is requested with a cursor has `HiddenUnseen` false, although hidden unseen galleries exist. So has every page in the `"all"` and `"hidden"` states and every page that is not empty.
- [x] A verdict view lists the same galleries with and without overrides. `"all"` or `"hidden"` with a verdict view, and an unknown `ScoreFilter` value, return `ErrInvalidQuery`.
- [x] A gallery projected from a feed entry (`projectFeed`) with a tag that an "always hide" override names is hidden. After its collected metadata without that tag is projected, `""` lists it.
- [x] Collector `GET /api/catalog?score_filter=bogus` gives 400 `invalid_query`. Through the real collector handler, local `GET /api/collector/catalog?score_filter=hidden` returns only the hidden galleries in a JSON page that has a `score_filter` object, and `score_filter=bogus` gives 400 `invalid_query`. A page without a filter in force has no `score_filter` field and no `score` field on its items.
- [x] `go vet ./...`, `go test ./...` and `gofmt -l .` are clean, with existing assertions unchanged.

## Resolution

`catalog.List` reads the overrides and applies the score filter (rules 1 to 3) per candidate in `score_filter.go`, with `Score` per page item and `HiddenUnseen`; `collectorapi` gains the score filter types, and both HTTP layers forward `score_filter`.
