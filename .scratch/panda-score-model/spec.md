# Score model for the Panda catalog

## Problem

The Panda catalog shows every gallery that passes the default filter. The user has told Tana, with verdicts and by passing pages, which galleries are worth a look, but nothing uses that. Each visit to the listing, and above all the unseen view, shows many galleries the user would reject at once, and the only way to thin them out is a hand-written default filter.

## Solution

The user opens an evaluation page from the Panda listing. It calculates a score model from the user's own evidence: verdicts, displayed galleries, and optionally Panda favorites. The page shows the weights, the evidence behind them, error rates measured on evidence the model did not learn from, and what a chosen threshold would hide. One action applies the model with its threshold. From then on the Panda listing hides galleries whose score is below the threshold, galleries with a "Not for me" verdict, and galleries that match an "always hide" override. A control beside the views switches between the filtered listing, all galleries, and only the hidden ones. The model, the threshold and the overrides live on the collector, so every device and browser tab sees the same state.

## Fit

Fits, with one small collector prefactor (ticket 01). Parts of the design that exist only because of how the code is built today:

1. **`catalog.Service.List` builds its predicate inline** (search, default filter, categories, expunged). The preview needs the same predicate without paging, to judge the listing "with the default filter applied". *Prefactor (ticket 01):* one private builder returns the predicate, its arguments and the page categories, and `List` calls it. No exported name changes and no test is edited.
2. **A display record holds only the gallery ID and the time**, in `panda_gallery_displays` and in the request that `goNext` sends. A display in the "hidden only" state and a display from before verdicts existed look like every other display. *Kept as an addition (tickets 04 and 10):* the record carries the score filter state, and a column `evidence` stores what follows from it. The migration sets it to `false` for displays older than the migration that created the verdict table, whose time `schema_migrations.applied_at` holds.
3. **`catalogPageQuery` merges one ordered scan per category, and each stops at its own limit.** *Fits:* the score condition is part of the shared predicate, so every branch tests each candidate, like the unseen probe. `verdictPageQuery` never gets it. A strict threshold or the "hidden only" state makes the scan long. That is accepted (see **Manual checks**).
4. **`summarizeTags` gives a card the first six tags.** *Fits:* the score and its strongest weights come from a separate lookup over all tags of the page's galleries, next to `markDisplayed` and `markVerdicts`.
5. **The default filter lives in Tana** (`internal/local/catalogfilter`). *Fits:* Tana's preview route loads it and sends it with the preview request, as `HandleCollectorCatalog` does for a catalog page.
6. **`saveVerdict` patches the loaded page and never loads the listing again.** A verdict changes whether rule 1 hides the gallery. *Workaround, kept:* the collector reports for each card which of rules 2 to 4 matches, without looking at the verdict, and the web applies rule 1 from the verdict the card shows.
7. **The toolbar's height is part of the measured page capacity.** The score filter control appears only after the first catalog page says that the filter is in force, so a narrow window can load the page a second time. *Kept:* the "Bypass default" button already appears the same way.
8. **The preview is the evaluation page's only read**, so every override change and every change of the favorites switch brings a new candidate. *Kept:* one request describes one consistent state, within the client's 30 s timeout and 4 MiB limit. The page holds the threshold fields in its own state, so a new candidate does not move them (behaviour 17).
9. **Tana and the collector are deployed independently.** An older collector ignores `score_filter` and has no score routes. *Kept:* the user updates both together (see **Out of scope**).

## Behaviour

Terms: *listing* is the Panda catalog page in Tana (`#/panda`) in any view or search. *Evidence* is what the calculation reads (behaviours 1 to 8). A *counted favorite* is a Panda favorite that counts as "Interesting" evidence. A *candidate* is the new model that the evaluation page shows before it is applied. The *active model* is the applied one. The score filter is *in force* when a model is active or at least one score override exists. The *filter state* of a listing is "on", "off" or "hidden only". Thresholds have one decimal place.

Evidence and calculation:

1. "Interesting" evidence is a gallery with the verdict "Interesting", or, when the favorites switch is on, a Panda favorite without a verdict (a counted favorite).
2. "Not for me" evidence is a gallery with the verdict "Not for me". This verdict wins over a favorite: such a gallery is never a counted favorite.
3. Neutral evidence is a displayed Panda gallery with no verdict that is not a counted favorite. It is weak evidence that the gallery is acceptable: it counts one tenth of an "Interesting" gallery.
4. A gallery counts one time, however many displays it has. A favorite that is in more than one Panda favorite collection, or in the collections of more than one Panda account, counts one time.
5. A display recorded in the "hidden only" state is not evidence. The gallery is still seen.
6. A display from the time before verdicts were possible is not evidence. That time ends when the collector applied the migration that created the verdict table.
7. The calculation reads the catalog metadata that is current when it runs: the gallery's tags, uploader and Panda gallery category, from collected or provisional Panda metadata. A gallery without a catalog entry gives no evidence.
8. An unknown uploader or category (an empty value) never gets a weight. The tags of such a gallery still count.
9. Each tag, uploader and category that at least 3 evidence galleries carry gets a weight. The weight is the logarithm of how much more often the value occurs in acceptable evidence than in "Not for me" evidence, compared with all evidence, with a prior that keeps the weight near zero when its evidence is small (formula in **Design**). A positive weight speaks for the gallery, a negative one against it.
10. The Panda gallery score is the plain sum of the weights of all of the gallery's tags, its uploader and its category. A value without a weight adds nothing. Tags that occur together count twice. This is accepted.
11. Sources in Tana libraries are not evidence. The calculation reads only the collector's verdicts, display records, favorites and catalog.
12. There is no model without verdicts, and none while one side of the evidence is missing: no "Not for me" evidence, or no "Interesting" and no neutral evidence.

Evaluation page:

13. The page is at `#/panda/score`, reached with "Score model" in the Options menu of the Panda listing, and it links back to the listing. On each visit it calculates a candidate from the current evidence and shows it beside the active model. Calculating or examining a candidate changes nothing in the listing.
14. The page shows the evidence counts: "Interesting" verdicts, "Not for me" verdicts, neutral galleries, counted favorites, the time of the last Panda favorite sync ("never" without one), and, with an active model, the number of verdicts given or changed after that model was calculated.
15. A switch, "Count Panda favorites as Interesting", calculates the model with or without favorites. Changing it calculates again and shows the new result, so the user can compare the two by switching. On a visit it starts with the setting of the active model, and off without one.
16. A table lists the candidate's weights by strength, the largest absolute weight first. Each row shows the tag (`namespace:value`), uploader or category, its weight, its evidence counts ("Interesting" including counted favorites, "Not for me", neutral), and its weight in the active model (a dash without one). The table shows 50 rows, and "Show more" adds 50. The page gets the 500 strongest weights and says how many the model has.
17. A threshold field (steps of 0.1) shows its effect at once, without a request and before anything is applied: the share of "Not for me", "Interesting" and neutral evidence galleries that the threshold hides, and the share of recent catalog galleries that the listing would hide with it. Beside the recent share, the page shows the share that the listing hides now. That share is 0 while the score filter is not in force. The field starts at the active model's threshold, and without an active model at a suggested value, which the page also names. A new calculation during the visit (the favorites switch, an override change, a retry) leaves the field's value as it is, so the user compares the results at one threshold. The field starts again only after a successful apply, threshold change or "Turn off".
18. The page shows two error rates for the candidate at the chosen threshold: the share of "Interesting" galleries that it would hide, and the share of "Not for me" galleries that it would show.
19. One button, "Apply model and threshold", makes the candidate the active model with the chosen threshold. The listing changes only then.
20. For the active model the page shows when it was calculated, whether it counts favorites, and its threshold. The user can save another threshold for it without a new calculation, and sees beside the field the share of recent catalog galleries the listing would then hide. That field keeps its value across calculations like the field of behaviour 17. "Turn off" removes the active model after a confirmation. Overrides stay.
21. Tana keeps only the active model. Applying a candidate replaces it, and turning it off deletes it. There is no history and no step back.
22. With fewer than 20 verdicts of either type, the page warns that weights and error rates from so little evidence are unreliable. The user can still apply the model.
23. With no verdicts, the page says how to give verdicts (the two controls on each card of the Panda listing) and offers no model: no table, no threshold and no apply. With verdicts but one side of the evidence missing, it offers no model either and says which side: no "Not for me" verdict yet, or no gallery that counts as acceptable yet. An active model and the overrides stay usable in all three cases.

Score overrides:

24. The user creates and removes overrides only on the evaluation page: with "Always hide" and "Never hide" on each row of the weights table, and with a form for a tag, uploader or category that is not in the table. The page lists the current overrides, each with "Remove".
25. A tag override names an existing catalog tag as `namespace:value`. An uploader override names the uploader exactly as Panda writes it. A category override names one of the Panda gallery categories. An empty or unknown value gets no override, and the page says why.
26. A tag, uploader or category has at most one override. A new rule for it replaces the old one.
27. An override takes effect in the listing with its next load, also when no model is active.
28. The preview includes the current overrides: every share of behaviours 17, 18 and 20 counts a gallery that an override always hides or never hides accordingly. After an override change the page calculates again.

Which galleries the score filter hides:

29. The score filter is in force when a model is active or at least one override exists. When it is not in force, no gallery is hidden, also not one with a "Not for me" verdict, and the listing is as it was before this feature.
30. The default filter is a separate layer and always applies first: the score filter only decides about galleries that pass it (all galleries when the default filter is bypassed).
31. For those galleries, the first rule that matches decides: (1) an "Interesting" verdict shows the gallery, a "Not for me" verdict hides it; (2) a "never hide" override that matches one of its tags, its uploader or its category shows it; (3) an "always hide" override that matches hides it; (4) with an active model, a score below the threshold hides it. Otherwise it shows.
32. A gallery with provisional Panda metadata gets its score, and its override matches, from the values the feed gave. After collection, the collected metadata decides. The collector stores no score: each catalog page computes it from the current catalog entry.
33. A hidden gallery is not on a page, so passing pages never makes it displayed, and it stays unseen. When a later model, override or metadata change shows it, it appears in the unseen view.

Score filter in the Panda listing:

34. A control beside the view control has three states: "Filter on" (apply the filter), "Filter off" (show all galleries) and "Hidden only" (show only the galleries the filter hides). The start state is "Filter on".
35. The control exists when the score filter is in force. The listing link holds the state as `score_filter=all` or `score_filter=hidden`, and nothing for "Filter on". Choosing a state drops the cursor. A link that holds a state shows the control even when the filter is not in force, so the user can always get back.
36. The control combines with the views "All" and "Unseen", and with the search, the categories, the default filter and its bypass, and "Include expunged". Changing any of them, and "Clear all filters", keeps the state.
37. The views "Interesting" and "Not for me" never apply the score filter and do not offer the control. Choosing one drops the state from the link, and a link with a verdict view and a state opens the verdict view.
38. "Filter off" and "Hidden only" show a "Not for me" gallery with its verdict, as every listing did before.
39. In the "Hidden only" state, a page the user passes becomes seen like every other page, but its displays are not evidence.
40. A verdict change never moves or removes a card on the current page, also when the new verdict hides the gallery, or shows it in the "Hidden only" state. The next load of the listing follows the rules.
41. With an active model, the hover details of each card, in every view, show the gallery's score, the threshold, and up to five of its strongest weights. When the score filter hides the gallery, they name the rule: the user's "Not for me" verdict, the "always hide" override with its tag, uploader or category, or the score below the threshold. When a "never hide" override decides, they name it.
42. An empty unseen view in the "Filter on" state does not say that the user has seen every gallery when the score filter hides unseen galleries that match the search and filters. At a link without a cursor, with or without a search or filters, its text says that the score filter hides the remaining unseen galleries, and it offers "Show hidden only". Its heading stays as it is. The empty page that ends a walk through the unseen view (a link with a cursor) does not change: it makes no such claim, and it leads to the newest unseen galleries, where the offer appears.
43. An empty listing in the "Hidden only" state without a cursor says that the score filter hides no gallery here, and offers "Filter on".
44. A catch-up also covers galleries that the score filter hides, and the confirmation says so. In the "Hidden only" state the confirmation says instead that it also covers galleries that the score filter does not hide.
45. The Panda lookup page does not change.

Failures:

46. If the collector is unreachable or not configured, the evaluation page shows the error with "Retry" and changes nothing.
47. A failed apply leaves the active model, its threshold and the listing as they were, and the page shows a message. An apply of a candidate that a later calculation replaced fails that way, with the advice to calculate again.
48. A failed override change leaves the overrides as they were, and the page shows a message.
49. A failed threshold change or "Turn off" leaves the active model as it was, and the page shows a message.

Guards:

50. The user applies exactly the model the page showed: the apply names the candidate, and the collector never applies a model from a later calculation in its place.
51. The error rates and the evidence shares come from evidence that the tested model did not learn from: each evidence gallery is scored by a model calculated without it.
52. The preview describes the listing as the user sees it: every share counts only galleries that pass the default filter and are not expunged, and the share the listing hides now is 0 while the score filter is not in force.
53. The model, the threshold and the overrides are one shared state on the collector for all devices and browser tabs. A listing or an evaluation page shows the state of its last load.

Local API:

54. `GET /api/collector/catalog` accepts `score_filter` (`all`, `hidden`; absent or empty means "on") and forwards it. Another value, or `all` or `hidden` with a verdict view, gives 400 `invalid_query`. When the filter is in force the page carries `score_filter` (with `threshold_tenths` for an active model, and `hidden_unseen` for behaviour 42), and each item carries `score` as far as it applies (see **Design**).
55. `POST /api/collector/catalog/displays` accepts an optional `"score_filter"` beside `"gallery_ids"`. `"hidden"` records displays that are not evidence. A value other than `""`, `"all"` or `"hidden"` gives 400 `invalid_batch`.
56. `POST /api/collector/catalog/score/overrides` with `{"kind","value","rule"}` returns 204. 400 `invalid_override` for an unknown kind or rule, an empty value, a tag without `namespace:value`, or an unknown category. 404 `tag_not_found` for a tag no catalog gallery ever had. `DELETE /api/collector/catalog/score/overrides/{id}` returns 204, also when no such override exists, and 400 `invalid_override` for an ID that is not a positive integer.
57. `POST /api/collector/catalog/score/preview` with `{"favorites": true|false|null}` returns the preview (see **Design**). Tana adds its default filter to the collector request.
58. `POST /api/collector/catalog/score/apply` with `{"candidate_id","threshold_tenths"}` returns 204, 409 `score_candidate_not_found` when the candidate is gone, 400 `invalid_threshold` outside -1000 to 1000. `PUT /api/collector/catalog/score/threshold` with `{"threshold_tenths"}` returns 204, 409 `score_model_not_active` without an active model, 400 `invalid_threshold`. `DELETE /api/collector/catalog/score/model` returns 204, also without an active model.
59. All score routes answer 503 `collector_not_configured` without a collector, 502 `collector_unauthorized` when the collector rejects Tana's token, and 502 `collector_unavailable` when it can't be reached or fails.

## Design

**Data flow.** The browser talks only to the local service. Every local route calls one `collectorapi.Client` method, which calls the collector route of the same name under `/api/catalog/...`, which calls one `catalog.Service` method. Non-test code in `internal/local/` reaches the collector only through `internal/collectorapi`. The preview is the page's only read: `POST /api/collector/catalog/score/preview` → Tana loads its default filter → `Client.PreviewScoreModel` → collector `POST /api/catalog/score/preview` → `Service.PreviewScoreModel`. After every change (apply, threshold, turn off, override) the page asks for the preview again.

Signatures, payloads, schema and the stated constraints are fixed. Query shapes, private helpers and test sequences are the implementer's. A line that starts with "Hint:" is optional.

### Collector schema (`internal/collector/storage/migrations`)

The catalog package uses pgx directly, so no sqlc step is needed. `storage_test.go` counts the embedded migrations itself. Each migration carries a comment on its purpose, in the style of `0004_panda_gallery_verdicts.sql`. Index names are the implementer's.

- `0005_panda_score_overrides.sql` (ticket 02): table `panda_score_overrides` with `id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `kind TEXT NOT NULL CHECK (kind IN ('tag', 'uploader', 'category'))`, `tag_id BIGINT REFERENCES catalog_tags (id)`, `value TEXT NOT NULL DEFAULT ''`, `rule TEXT NOT NULL CHECK (rule IN ('always_hide', 'never_hide'))`, `created_at TIMESTAMPTZ NOT NULL`. Checks: a tag row has `tag_id` and an empty `value`; an uploader or category row has no `tag_id` and a non-empty `value`. Unique: `tag_id` among tag rows, `(kind, value)` among the others. `catalog_tags` rows are never deleted or renumbered (the projection only inserts), so the reference survives every projection change. No reference to a gallery row (ADR 0019).
- `0006_panda_display_evidence.sql` (ticket 04): `panda_gallery_displays` gains `evidence BOOLEAN NOT NULL DEFAULT true`, and a one-time update sets it to `false` for every display older than `schema_migrations.applied_at` of version 4. On a database that gets migrations 4 and 6 in one transaction, that time is the transaction's, so every existing display is marked.
- `0007_panda_score_models.sql` (ticket 05): table `panda_score_models` with `id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `active BOOLEAN NOT NULL DEFAULT false`, `favorites BOOLEAN NOT NULL`, `calculated_at TIMESTAMPTZ NOT NULL`, `threshold_tenths INTEGER`, a check that exactly the active row has a threshold, and a unique partial index that allows one active row. Table `panda_score_weights` with `model_id BIGINT NOT NULL REFERENCES panda_score_models (id) ON DELETE CASCADE`, `kind`, `tag_id`, `value` with the checks of the override table, `weight DOUBLE PRECISION NOT NULL`, and `interesting`, `not_for_me`, `neutral` as `INTEGER NOT NULL`. Unique: `(model_id, tag_id)` among tag rows and `(model_id, kind, value)` among the others. These two indexes serve the score lookup for one gallery.

### Collector: `internal/collector/catalog`

All score code stays in this package: it needs `List`'s predicate, the seen tables and the tag tables, and a second package would import this one and be imported by it.

**Ticket 01.** A private builder holds what `List` does today between `searchNamespaces` and the `IncludeExpunged` condition, and returns the predicate on alias `g`, the bound arguments and the page categories. `List` calls it and then adds the unseen conditions and the cursor, as today.

**Overrides (ticket 02).** New file, with:

- `var ErrInvalidScoreOverride`, `var ErrTagNotFound`.
- `func (s *Service) SetScoreOverride(ctx context.Context, kind, value, rule string) error`. Kinds `tag`, `uploader`, `category`; rules `always_hide`, `never_hide` (constants in `collectorapi`). The value is trimmed. A tag value is cut at its first colon into namespace and value and matched on `catalog_tags (namespace, value_lower)` with the folding of `replaceTags` (lower case, `_` to space). No colon, an empty part, an unknown kind or rule, or an empty value return `ErrInvalidScoreOverride`. A tag that is not in `catalog_tags` returns `ErrTagNotFound`. A category goes through `panda.NormalizeCategories`, and its error is returned as `ErrInvalidScoreOverride`. An uploader is stored as given and matches `catalog_galleries.uploader` exactly. An existing override for the same tag, uploader or category gets the new rule (behaviour 26).
- `func (s *Service) RemoveScoreOverride(ctx context.Context, id int64) error`. `id <= 0` returns `ErrInvalidScoreOverride`. A missing row is not an error.
- A private reader of all overrides for `List` and the preview, giving each one's ID, kind, rule, tag ID and label. The label is `namespace:value` for a tag (the stored `value` spelling), and the value for the others.

**The score filter in `List` (ticket 03, rule 4 in ticket 06).** `collectorapi.CatalogOptions.ScoreFilter` is `""` (on), `"all"` or `"hidden"`.

- Another value returns `ErrInvalidQuery`, and so does `"all"` or `"hidden"` with `options.Verdict`. Both are checked before the transaction starts.
- Inside its transaction `List` reads the overrides and (ticket 06) the active model's ID and threshold. The filter is in force when either exists.
- With a verdict view, or when the filter is not in force, the page query is today's, except that `"hidden"` without a filter in force returns an empty page.
- Otherwise a gallery is *hidden* by the rule order of behaviour 31: its verdict first, then a "never hide" match, then an "always hide" match, then (ticket 06) `floor(score * 10) < threshold_tenths`. `""` lists the galleries that are not hidden, `"hidden"` only the hidden ones, and `"all"` both.
- Constraints on that condition:
  - It is part of the predicate that `catalogPageQuery` copies into each category branch, so every branch tests each candidate in catalog order and stops at its limit.
  - It tests each candidate by key: the gallery's verdict, its own rows in `catalog_gallery_tags`, its uploader and category, and the active model's weights for exactly those values. It never starts from the galleries of a tag or from the weights of a model. Hint: `tagMatcher` does this for an excluded tag, with the tag IDs read beforehand and bound as an array.
  - The score counts all tags of the gallery, its uploader and its category. A value without a weight counts 0. An empty uploader or category matches no weight and no override.
  - The threshold is compared in tenths everywhere (`floor(score * 10)` against an integer). One SQL score expression, for a gallery and a model ID, serves the page condition, the per-card lookup and the recent sample of ticket 07, so the listing and the preview can't disagree at a bucket border.
- `CatalogResult.ScoreFilter` is set when the filter is in force, in every view. `HiddenUnseen` is set only on a request without a cursor, in the unseen view and the `""` state, when the page is empty and at least one hidden gallery passes the same search, filters and unseen conditions. A request with a cursor never sets it. The check stops at the first such gallery.
- When the filter is in force, a lookup over the page's items (next to `markVerdicts`, never over every candidate) sets `CatalogItem.Score` in every view: `Rule` is the first of rules 2 to 4 that matches, without regard to the verdict (`never_hide`, `always_hide`, `below_threshold`, or empty), `Override` the label of the override that decided (the first by kind order tag, uploader, category, then label), and from ticket 06 `Value` and up to five `Weights`, the strongest by absolute weight among all tags of the gallery, its uploader and its category. An item with nothing to report has no `Score`.
- `CatchUp`, `SetVerdict`, `RemoveVerdict` and the projection don't change.

**Display evidence (ticket 04).** `func (s *Service) RecordDisplays(ctx context.Context, ids []int64, scoreFilter string) error`: `"hidden"` stores the displays with `evidence = false`, `""` and `"all"` with `true`, another value returns `ErrInvalidDisplays`. Existing callers pass `""`. `markDisplayed` and the unseen probe keep reading every display. The display that `SetVerdict` records is evidence.

**Calculation (ticket 05).** `func (s *Service) PreviewScoreModel(ctx context.Context, options collectorapi.ScorePreviewOptions) (collectorapi.ScorePreview, error)` runs in one transaction at READ COMMITTED (ADR 0018).

- *Evidence.* One row per gallery ID that has a catalog entry (behaviour 7), with its class by behaviours 1 to 6: a gallery with a verdict; with `Favorites`, a gallery in `favorites` (any collection, counted once) without a verdict; a gallery with at least one display that has `evidence`, in neither group. The transaction reads the evidence once: the counts it reports, the weights and (ticket 08) the held-out scores all come from that one reading.
- *Counts.* Per class, the number of evidence galleries, and the number of those that carry each tag (all rows of `catalog_gallery_tags`), each non-empty uploader and each non-empty category.
- *Weights.* Pure functions in this package, tested without a database, turn counts into weights. Constants: `NeutralWeight = 0.1`, `PriorStrength = 10`, `MinSupport = 3`, `FewVerdicts = 20`. With `I`, `N`, `U` the numbers of "Interesting", "Not for me" and neutral evidence galleries, and `i`, `n`, `u` the numbers of those that carry the value: `A = I + NeutralWeight*U`, `R = N`, `a = i + NeutralWeight*u`, `r = n`, `p = A / (A + R)`, and `weight = ln((a + PriorStrength*p) / (r + PriorStrength*(1-p))) - ln(A / R)`. A value gets a weight only when `i + n + u >= MinSupport`. With `A = 0` or `R = 0` there is no model.
- *Candidate.* The preview deletes every model that is not active and stores its own: `favorites`, `calculated_at` as the database's time, and all weights with their counts. So a preview leaves no candidate but its own behind (behaviours 21 and 50). Storing the weights must not cost one statement per weight: a model has thousands. A weight's label follows the label rule of the overrides, so the page can match a weight row with an override by kind and label.

Result in ticket 05: `Favorites`, `Evidence` (the four counts, `FavoritesSyncedAt` as `max(favorite_categories.synced_at)`, `FewVerdicts` when either verdict count is under `FewVerdicts`), `Candidate` (ID, time, the 500 strongest weights ordered by absolute weight, then kind, then label, and `WeightCount`) or `NoModel`, and `Overrides`. `NoModel` is `no_verdicts` when no evidence gallery has a verdict, else `no_not_for_me` when there is no "Not for me" evidence, else `no_acceptable` when there is no "Interesting" and no neutral evidence. An invalid default query or category returns the errors `List` returns for them, from ticket 05 on, although the default filter is first used in ticket 07.

**Apply, threshold, turn off (ticket 06).**

- `var ErrScoreCandidateNotFound`, `var ErrInvalidScoreThreshold`, `var ErrNoActiveScoreModel`.
- `func (s *Service) ApplyScoreModel(ctx context.Context, candidateID int64, thresholdTenths int) error` makes the candidate the active model with the threshold and deletes the model that was active, in one transaction. A threshold outside -1000 to 1000 returns `ErrInvalidScoreThreshold`. When `candidateID` is not a stored candidate, it returns `ErrScoreCandidateNotFound` and has changed nothing.
- Apply is a read-then-write sequence, so it locks what it depends on (ADR 0018). Its transaction first locks `panda_score_models` in `SHARE ROW EXCLUSIVE` mode, as `LockLibraries` in `internal/local/library` locks its table. Applies then run one after the other; a preview, a threshold change and a turn-off wait for an apply, and an apply waits for a preview that has written; catalog pages keep reading. Under the lock it checks that the candidate exists and is not active, and only then deletes and promotes. A row lock alone can't do this: the first apply has no active row to lock, and two applies of different candidates would meet at the one-active-row index.
- `func (s *Service) SetScoreThreshold(ctx context.Context, thresholdTenths int) error` changes the active row in one statement, and returns `ErrNoActiveScoreModel` when there is none.
- `func (s *Service) RemoveScoreModel(ctx context.Context) error` deletes the active model in one statement. Candidates and overrides stay.

**Preview against the active model and the recent catalog (ticket 07).**

- `options.Favorites == nil` means the active model's setting, and `false` without one. `ScorePreview.Favorites` reports the setting used.
- `Active` (calculated time, favorites, threshold), `Evidence.NewVerdicts` (evidence verdicts with `given_at` after the active model's `calculated_at`; nil without an active model), and `ActiveWeight` on each returned weight.
- `Recent`: the newest `RecentSampleSize = 2000` galleries in catalog order that pass the default filter of the request and are not expunged. Their predicate comes from ticket 01's builder, called with only the default filter. Each is classed by the first rule of behaviour 31 that matches: `Interesting`, `NotForMe`, `NeverHidden`, `AlwaysHidden`, and the rest counted by `floor(score * 10)` into one histogram under the candidate's weights and one under the active model's.

**Held-out effect (ticket 08).** The fold of an evidence gallery is `gallery_id % 5`. For each fold, weights are calculated from the evidence outside the fold, by the weight functions of ticket 05, and the evidence galleries in the fold are scored with them. A fold whose training evidence lacks one side scores its galleries 0. Fold weights are not kept after the transaction. `Candidate.HeldOut` gives, per class, among the evidence galleries that pass the default filter and are not expunged: `Total`, `AlwaysHidden` and `NeverHidden` (rules 2 and 3, never the verdict), and a histogram of the held-out scores of the rest. `Candidate.SuggestedThresholdTenths` comes from a pure function: the highest threshold within the "Interesting" histogram's range that hides at most `SuggestedInterestingLoss = 0.05` of the held-out "Interesting" galleries, the histogram's first bucket when none does, and 0 without scored "Interesting" galleries.

Test seams: package tests on `pgtest` in new test files, with `openCatalog`, `retain`, `projectFeed`, `setVerdict`, `unseenPage`, `assertCatalogIDs` and direct SQL for favorites (as `TestCatalogFactsUseLatestCurrentFavoriteEvenWithoutMetadata` seeds them) and for backdated displays. `TestConcurrentCatchUpsKeepTheNewestPosition` shows how a test holds calls at a lock. The weight and threshold functions get plain table tests.

### `internal/collectorapi`

- Ticket 02: `ScoreKindTag`, `ScoreKindUploader`, `ScoreKindCategory`, `ScoreRuleAlwaysHide`, `ScoreRuleNeverHide`; `type ScoreOverride struct{ ID int64; Kind, Label, Rule string }` (JSON `id`, `kind`, `label`, `rule`); `func (c *Client) SetScoreOverride(ctx, kind, value, rule string) error` (`POST /api/catalog/score/overrides`, 204) and `func (c *Client) RemoveScoreOverride(ctx, id int64) error` (`DELETE .../{id}`, 204). Other statuses return `downloadResponseError`, as everywhere below.
- Ticket 03: `ScoreFilterAll = "all"`, `ScoreFilterHidden = "hidden"`, `func ValidScoreFilter(string) bool` (also true for `""`). `CatalogOptions.ScoreFilter`, sent as `score_filter` on every request. `CatalogResult.ScoreFilter *CatalogScoreFilter` (JSON `score_filter,omitempty`) with `ThresholdTenths *int` (`threshold_tenths,omitempty`, ticket 06) and `HiddenUnseen bool` (`hidden_unseen,omitempty`). `CatalogItem.Score *CatalogItemScore` (`score,omitempty`) with `Rule string` (`rule,omitempty`), `Override string` (`override,omitempty`), and in ticket 06 `Value *float64` (`value,omitempty`) and `Weights []CatalogScoreWeight` (`weights,omitempty`; `kind`, `label`, `weight`).
- Ticket 04: `RecordCatalogDisplays(ctx, ids []int64, scoreFilter string)` sends `score_filter` only when it is not empty, so today's request body is unchanged for `""`.
- Ticket 05: `ScorePreviewOptions{ Favorites *bool; DefaultQuery string; DefaultCategories []string }` (JSON `favorites`, `default_query`, `default_categories`) and `ScorePreview`. JSON of the full preview after ticket 08:
  ```json
  {
    "favorites": false,
    "evidence": {"interesting": 40, "not_for_me": 120, "neutral": 5000, "counted_favorites": 0,
                 "favorites_synced_at": "2026-11-01T10:00:00Z", "new_verdicts": 12, "few_verdicts": false},
    "candidate": {"id": 7, "calculated_at": "...", "favorites": false, "weight_count": 3100,
                  "suggested_threshold_tenths": -8,
                  "weights": [{"kind": "tag", "label": "female:glasses", "weight": -1.42, "interesting": 0,
                               "not_for_me": 31, "neutral": 12, "active_weight": -1.1}],
                  "held_out": {"interesting": {"total": 38, "always_hidden": 0, "never_hidden": 1,
                                               "scores": {"first_bucket": -31, "counts": [1, 0, 2]}},
                               "not_for_me": {}, "neutral": {}}},
    "no_model": "",
    "active": {"calculated_at": "...", "favorites": false, "threshold_tenths": -5},
    "overrides": [{"id": 3, "kind": "uploader", "label": "Some uploader", "rule": "always_hide"}],
    "recent": {"total": 2000, "interesting": 4, "not_for_me": 20, "never_hidden": 3, "always_hidden": 40,
               "candidate": {"first_bucket": -40, "counts": []}, "active": {"first_bucket": -35, "counts": []}}
  }
  ```
  `candidate` and `active` are `null` when there is none. `no_model` is `no_verdicts`, `no_not_for_me` or `no_acceptable` without a candidate, and omitted with one. `favorites_synced_at`, `new_verdicts` and `active_weight` are `null` when unknown, `overrides` and `weights` are never `null`, and the two histograms in `recent` are `null` without their model. In a histogram, `counts[i]` is the number of galleries with `floor(score * 10) == first_bucket + i`. A threshold of `t` tenths hides `always_hidden` plus the counts of all buckets below `t`.
- Ticket 09: `func (c *Client) PreviewScoreModel(ctx, ScorePreviewOptions) (ScorePreview, error)`, `ApplyScoreModel(ctx, candidateID int64, thresholdTenths int) error`, `SetScoreThreshold(ctx, thresholdTenths int) error`, `RemoveScoreModel(ctx) error`.

### Collector: `internal/collector/httpapi`

Routes next to the catalog routes in `routes.go`, behind the same token check:

- Ticket 02: `POST /api/catalog/score/overrides` (`{"kind","value","rule"}`, 204) and `DELETE /api/catalog/score/overrides/{id}` (204; an ID that is not an integer is an invalid override). `catalogError` maps `ErrInvalidScoreOverride` to 400 `invalid_override` and `ErrTagNotFound` to 404 `tag_not_found`.
- Ticket 03: `HandleCatalog` passes `score_filter` to the options unparsed.
- Ticket 04: `HandleRecordCatalogDisplays` reads the optional `score_filter`.
- Ticket 05: `POST /api/catalog/score/preview` with the options as body, 200 with the preview.
- Ticket 06: `POST /api/catalog/score/apply` (`{"candidate_id","threshold_tenths"}`), `PUT /api/catalog/score/threshold` (`{"threshold_tenths"}`), `DELETE /api/catalog/score/model`, each 204. `catalogError` maps `ErrScoreCandidateNotFound` to 409 `score_candidate_not_found`, `ErrNoActiveScoreModel` to 409 `score_model_not_active`, `ErrInvalidScoreThreshold` to 400 `invalid_threshold`.
- Test seam: validation tables in `catalog_test.go`, like `TestCatalogVerdictValidation`.

### Local service: `internal/local/httpapi`

New file for the score routes, registered next to the catalog routes. Every handler checks `catalogCollectorConfigured`, decodes with `server.DecodeJSON`, forwards, and reports errors through `writeCatalogCollectorError`, which gains the pass-through of 400 `invalid_override` and `invalid_threshold`, 404 `tag_not_found`, and 409 `score_candidate_not_found` and `score_model_not_active`. The collector validates; the local handlers only validate the override ID in the path (400 `invalid_override`, not the 404 of `pathID`).

- Ticket 02: `HandleCollectorSetScoreOverride`, `HandleCollectorRemoveScoreOverride`.
- Ticket 03: `HandleCollectorCatalog` forwards `score_filter`.
- Ticket 04: `HandleCollectorRecordCatalogDisplays` accepts and forwards `score_filter`, and answers 400 `invalid_batch` itself for a value `collectorapi.ValidScoreFilter` rejects.
- Ticket 09: `HandleCollectorScorePreview(client, defaults *catalogfilter.Store)` reads `{"favorites"}`, loads the default filter (errors as in `HandleCollectorCatalog`) and sends both. `HandleCollectorApplyScoreModel`, `HandleCollectorSetScoreThreshold`, `HandleCollectorRemoveScoreModel`.
- Test seam: `catalog_test.go`, running the real collector handler behind `httptest.NewServer`, as `TestLocalCatalogVerdicts` does.

### Web

Tickets 10 to 13 carry the wording and the layout. Fixed here:

- `web/src/navigation.ts` (ticket 10): `export type PandaScoreFilter = 'apply' | 'all' | 'hidden'`, `PandaListing.scoreFilter`. The route reads `score_filter=all|hidden`, and `'apply'` for anything else and for every verdict view. `pandaListingLink` writes `score_filter` before the view parameter, never for `'apply'` and never with a verdict view. Ticket 11 adds the route kind `'panda-score'` for `/panda/score`.
- `web/src/panda/api.ts` (ticket 10): `listPandaCatalog` sends `score_filter` for `'all'` and `'hidden'` outside verdict views. `recordPandaDisplays(galleryIDs, scoreFilter)` adds `score_filter: 'hidden'` to the body only in that state. `PandaCatalogResult.score_filter?` and `PandaGallery.score?` mirror the JSON above. All new fields are optional, so existing fixtures stay valid.
- `web/src/panda/PandaCatalog.tsx` (ticket 10): every link and request of the listing carries `scoreFilter`. A second `SegmentedControl` ("Score filter") follows the view control, when `result.score_filter` exists or the state is not `'apply'`, and never in a verdict view. `goNext` passes the state to the record. Of the three cases of today's empty unseen view, only the two without a cursor change (behaviour 42).
- `web/src/panda/PandaCard.tsx` (ticket 10): `PandaDetails` takes the page's `score_filter` and shows behaviour 41. The hidden rule is derived in the browser: the card's verdict first, then `score.rule`.
- `web/src/panda/score.ts` (tickets 11 to 13): the preview types, one request function per local score route, the messages for the new error codes, and three pure functions: `hiddenShare(effect, thresholdTenths)`, `recentHiddenShare(recent, histogram, thresholdTenths)`, and `currentHiddenShare(preview)`, the share of the recent sample that the listing hides now. `currentHiddenShare` is 0 when `active` is null and `overrides` is empty. With overrides and no active model it counts verdicts and overrides. With an active model it also counts the active histogram below the active threshold.
- `web/src/panda/PandaScorePage.tsx` (tickets 11 to 13), rendered by `App.tsx` for `'panda-score'`, with the side navigation's Panda entry active, laid out like `PandaLookupPage`. One `useMutation` serves all changes of the page. The page keeps the values of its two threshold fields in tenths, in its own state. A value is set from the first preview that can supply it, and again from the first preview after a successful apply, threshold change or turn-off. No other preview changes it.
- Test seams: new `web/src/panda/PandaScoreFilter.test.tsx` (ticket 10) renders `<App />` with its own stubbed `fetch` and the `useGalleryGrid` mock, as `PandaVerdicts.test.tsx` does. New `web/src/panda/PandaScorePage.test.tsx` (ticket 11) renders the page with a stubbed `fetch`, and tickets 12 and 13 add their cases to it.

**Tests in every package.** In tickets 02 to 13, existing assertions stay unchanged, new cases may be added to existing test files, and call sites may gain an argument. Only the prefactor ticket 01 leaves every test file unedited.

**Risky choices.**

- **The score is computed per candidate inside the page query**, not stored per gallery. It always follows the current metadata, model and overrides (behaviours 32 and 33), at the price of a long scan when most candidates are hidden. A stored score per gallery would need an update on every projection and every apply.
- **The evidence cutoff comes from `schema_migrations.applied_at`.** It is the only record of when verdicts became possible on this collector.
- **A candidate is stored on the collector and applied by ID.** The page never sends weights back. A second calculation, from any tab, deletes the first candidate, and its apply then fails with a clear message.
- **An apply takes a table lock on `panda_score_models`.** The table holds a few rows and is written only from the evaluation page. An apply that meets a running calculation waits for it, and then fails with `score_candidate_not_found` when that calculation deleted its candidate.
- **A catalog page reads the active model and then runs its page query as separate statements.** A page that loads while an apply commits can meet the replaced model's ID, whose weights are gone, and then scores every gallery 0 for that one page. Accepted: the next load is right, and an apply is a rare, manual action.

## Assumptions

- **The score filter is in force only when a model is active or an override exists**, and only then does a "Not for me" verdict hide its gallery. Rejected: the verdict rule always hides. The brief gives the control only with a model or an override, so hidden galleries would have no control that shows them.
- **The share the listing hides now is 0 while the filter is not in force**, also when recent galleries have a "Not for me" verdict. Rejected: counting those verdicts, which describes a listing the user does not see.
- **Weight formula and constants** (behaviour 9 and **Design**): a smoothed log odds ratio with a prior of 10 galleries, neutral evidence at one tenth, at least 3 galleries per value. Rejected: unsmoothed ratios (one gallery gives an extreme weight) and a trained regression (not a simple sum the user can read).
- **Neutral evidence counts on the acceptable side at weight 0.1.** Rejected: a class of its own with separate weights.
- **The evidence shares of behaviour 17 are the held-out numbers of behaviour 18**, by 5-fold cross-validation on `gallery_id % 5`. Rejected: shares measured on the galleries the model learned from, which flatter it, and a single held-out fifth, which wastes four fifths of few verdicts.
- **The class shares ignore rule 1.** Rule 1 decides every gallery with a verdict in the real listing, which would make the "Not for me" share 100 % and the "Interesting" share 0 % at every threshold.
- **"Recent catalog galleries" are the newest 2000 that pass the default filter**, expunged excluded. Rejected: a time window, which depends on feed gaps.
- **The threshold is stored and compared in tenths**, with a range of -100.0 to 100.0.
- **The start threshold without an active model** hides at most 5 % of the held-out "Interesting" galleries. Rejected: 0, which means something different for every model.
- **The threshold fields keep their values across the calculations of one visit**, and the page names the suggested threshold of the shown candidate beside the field. Rejected: starting again with each candidate. Then the comparison with and without favorites runs at two thresholds, and an override change throws away what the user entered.
- **One stored candidate.** Rejected: sending the weights back with the apply (large, and Tana would have to trust them), and recalculating at apply time (then the applied model could differ from the shown one).
- **Applies are serialized with a table lock.** Rejected: a row lock on the candidate plus a lock on the active row, which can't serialize the first apply, when no active row exists.
- **Counted favorites are favorites without any verdict**, across all accounts and collections, like `Service.Facts`. The favorites table only holds the newest version of a gallery, so a verdict on an older version does not stop the newest version from counting.
- **"Last Panda favorite sync" is the newest `synced_at` of any favorite collection.** Rejected: the oldest, which is the stricter bound on staleness but is not "the last sync".
- **The favorites switch starts with the active model's setting**, off without one.
- **"Few verdicts" means fewer than 20 of either verdict type.** Counted favorites don't lift the warning.
- **Evidence that lacks one side gives no model, and the collector names the side** (`no_not_for_me`, `no_acceptable`), beside `no_verdicts`. Rejected: the page derives the side from the counts, which copies the rule for "acceptable" into the browser.
- **The display that a verdict records is always evidence**, also when the verdict was given in the "Hidden only" state and later removed.
- **Displays in the "Filter off" state are evidence**, as the brief excludes only "hidden only".
- **A tag override needs an existing catalog tag, written `namespace:value`.** Rejected: free text, which could never match. **An uploader override is free text matched exactly**, because the catalog has no uploader index to check it against. The brief's "unknown uploader or category" is read as the empty value.
- **The weights table shows the 500 strongest weights of the candidate.** A value that only the active model weighs is not listed. The override form covers every value that is not listed.
- **The threshold change of the active model shows only the recent catalog share.** Class shares for the active model would come from evidence it learned from.
- **The page calculates on every visit, switch change and state change**, with no "Calculate" button.
- **"Turn off" asks for confirmation**, because there is no step back.
- **Only the empty unseen page without a cursor gets the score filter text and "Show hidden only".** The page with a cursor ends a walk through the view. Its text makes no claim about seen galleries, and its link leads to the page that carries the offer. Rejected: the offer on both, where "the unseen galleries that remain" would ignore unseen galleries newer than the cursor.
- **Control labels** "Filter on", "Filter off", "Hidden only" (the view control already has "All"), the link `score_filter=all|hidden`, the route `#/panda/score`, and all page and message wording are mine.
- **"Clear all filters" keeps the filter state**, like the view.
- **The hover details show score data in the verdict views too.**
- **Terms.** `CONTEXT.md` gains the five terms of the brief, and **Displayed Panda gallery** gains that a display records whether it is evidence. "Evidence", "counted favorite", "candidate", "in force" and "filter state" are defined in this spec only. ADR 0019 gains a paragraph instead of a new ADR.
- **Tana and the collector are updated together.**

## Out of scope

- A history of models, a step back, and a comparison of more than candidate and active model.
- Automatic recalculation. The active model changes only when the user applies a candidate.
- Charts of the score distributions.
- Tag completion in the override form, and overrides for a single gallery.
- Weights for anything but tags, uploader and category (title words, page count, language as a special case).
- Scores or the score filter in the local gallery listing, on the Panda lookup page, and in Panda enrichment.
- A cap on the neutral evidence, and storing scores per gallery.
- Mixed versions of Tana and the collector.

## Manual checks

- **Weights against real evidence.** Open the evaluation page on the real collector. Good: the top rows are tags and uploaders the user recognises as liked or disliked, not values seen three times. Tune `PriorStrength` (10), `NeutralWeight` (0.1) and `MinSupport` (3).
- **Start threshold.** Good: the suggested threshold hides a useful share of "Not for me" without hiding a visible share of "Interesting". Tune `SuggestedInterestingLoss` (0.05).
- **Few-verdicts warning.** Good: it goes away about when the error rates stop jumping between calculations. Tune `FewVerdicts` (20) and the number of folds (5).
- **Recent share.** Good: the predicted share matches what the first pages of the listing then hide, and "now" reads 0 % before the first apply when no override exists. Tune `RecentSampleSize` (2000).
- **Preview time.** Time the preview on the real collector. Good: a few seconds. The client's 30 s timeout is the ceiling; nearing it calls for a cap on neutral evidence.
- **Apply during a calculation.** Open the evaluation page in two tabs and apply in one while the other calculates. Good: the apply answers within the time of that calculation, with success or the advice to calculate again, and never with a collector error.
- **Listing time with a strict threshold and in "Hidden only".** Run the page query with `EXPLAIN (ANALYZE, BUFFERS)` on the real database, with one and with several categories. Good: an ordered index scan that probes the score per candidate and stops at the limit, no sort or scan of the whole catalog, and a first page in about a second.
- **Evidence cutoff.** On the real collector, compare `SELECT applied_at FROM schema_migrations WHERE version = 4` with the day verdicts shipped, and count the displays the migration marked.
- **Toolbar.** With the filter in force, the score filter control sits beside the view control, wraps without overflow at phone width, and the listing loads at most once more when it appears.
- **Hover details.** The score lines fit the tooltip and read well for a gallery with long tag names. Tune the number of weights (5).
- **Evaluation page layout.** The sections read in order on a phone and a desktop window, the weights table scrolls inside the page, and the effect numbers change as the threshold is stepped. Check whether a number field is enough or a slider is wanted. Tune the table's page of 50 rows and the 500 weights a preview returns.
- **Comparing calculations.** Enter a threshold, change the favorites switch, then add an override. Good: the threshold field keeps the entered value each time, the shares change with the new calculation, and the suggested value beside the field shows what the new model would start with.
- **Empty unseen view.** With every remaining unseen gallery hidden, walk the unseen view to its end with Next. Good: the end page reads as before, "Newest unseen galleries" leads to the score filter message, "Show hidden only" reads naturally there, and catching up from "Hidden only" shows the adapted confirmation.
