# Favorites-only Panda score model

## Problem

The Panda score model learns from three kinds of evidence: verdicts ("Interesting", "Not for me"), displayed galleries, and optionally Panda favorites. The user must give verdicts card by card to get a model at all, and a model needs "Not for me" verdicts before it can exist. The verdict controls sit on every card, two more views sit in the listing, and a "Not for me" verdict hides a gallery whatever its score says. The user already says what they like by favoriting on Panda, and already says what they dislike with pinned weights and score overrides.

## Solution

The Panda score model learns only from the user's Panda favorites. A value (tag, uploader, Panda gallery category) weighs more the more frequent it is in the favorites than in the Panda catalog, both limited to the default filter. A calculated weight is never negative: the user expresses dislike only with pinned weights and score overrides. Verdicts are gone: no controls on the cards, no verdict views, no stored verdicts, and no verdict rule in the score filter. A display only makes a gallery seen. The score page shows the favorites behind the model, how many favorites and catalog galleries carry each weighted value, and how a threshold treats favorites that the weights did not learn from. Stored models are deleted with the update, so the score filter stays off until the user applies a new model.

## Fit

Fits without a prefactor. What the design carries only because of how the code is built today:

1. **Collector tests get their models by giving verdicts and applying a preview** (`seedVerdicts`, `seedScoreEvidence`, `applyCandidate` in `internal/collector/catalog`, and `service.SetVerdict` in `internal/local/httpapi/catalog_test.go`). About 30 listing, pin, author bonus and summary-tag tests expect negative calculated weights, which can no longer exist, and the lifecycle tests (apply, replace, locks) take their candidates from verdict previews. *Handled in ticket 02, as test work inside a behaviour ticket:* these tests store their models and candidates directly, so only tests of the calculation go through a preview. It is not a `Prefactor:` ticket, because it edits tests.
2. **The preview is the score page's only read, so today every pin, override and bonus change calculates again**, inside the lock that apply, a threshold change and turn-off wait on. The new calculation counts catalog galleries per value, the first preview work that grows with the catalog. Dislike now goes only through pins and overrides, so these edits become the page's main use and must not each recount the catalog. *Kept, with an addition (tickets 04 and 08):* the candidate stores the counts it was calculated from, and a change that alters no calculated weight measures the stored candidate again. That is state that outlives a request: two count columns per stored weight and two totals per model (the table stores three count columns per weight today), and a `candidate_id` in the preview request. *Not done:* a read of the page's state that is separate from the calculation. It would split `collectorapi.ScorePreview`, both HTTP layers and most of `PandaScorePage.test.tsx`, a large prefactor. *Accepted risk:* the time of the one calculation per visit is not measured (**Assumptions**, **Manual checks**).
3. **The score page and its test file are too large to share a ticket with the calculation.** `PandaScorePage.tsx` and `PandaScorePage.test.tsx` alone are about 2,000 lines, and the calculation ticket reads about as much Go. So the calculation (ticket 04) and the page (ticket 05) are a pair of adjacent tickets, not one tracer bullet, and two collector tickets fill numbers that the page already shows. The checks stay green throughout, because the web tests mock `fetch`. The verdict removal is also spread over tickets by layer. What a person who runs the app on the feature branch sees in between:
   - *After ticket 01, before ticket 02:* the collector still hides galleries with a "Not for me" verdict, and the card has no control to change that and names no rule for it.
   - *After ticket 02, before ticket 04:* the recent sample of the score page still counts galleries with a "Not for me" verdict as hidden, while the listing no longer hides them, so the page's "Hides … %" numbers are too high by their share.
   - *After ticket 04, before ticket 05:* the score page throws when it renders a preview, because it reads evidence and weight fields that ticket 04 removed. This is the only window in which a page throws. The listing is not affected.
   - *After ticket 05, before ticket 06:* the page works. Its threshold block shows "Hides 0 % of the held-out favorites" and "Suggested: 0.0", because the collector sends empty held-out counts until ticket 06. The recent catalog line is right.
   - *After ticket 05, before ticket 07:* the evidence section has no "new favorites" row, because the collector sends `null` until ticket 07.
   The feature is merged only as a whole.
4. **`server.DecodeJSON` rejects unknown fields**, so a request body changes on every side in one ticket: the preview body in tickets 03 and 08, the display record in ticket 10.
5. **The local route tests run the collector's handler in process**, so a changed `collectorapi` signature and its local route share a ticket (tickets 01, 03, 08, 10).
6. **The listing's predicate leaves out expunged galleries unless asked.** *Fits:* the scope of the calculation asks for them (**Assumptions**); the recent sample keeps the listing's predicate.
7. **Nothing stores when the collector first saw a favorite.** `favorites.added_at` is Panda's time, and `favorite_observations` holds one row per gallery ever observed as a favorite, without a time. *Addition:* a column `observed_at` on `favorite_observations`, filled by its default, with no code change in the `favorites` package.
8. **`favorite_authors` counts every favorite of an author, the held-out one too.** *Fits:* the table stores the count, so a held-out score can ask whether the author stays a favorite author without one favorite (behaviour 26).
9. **Applied migrations are never edited.** Migrations 0004 to 0007 keep their comments about verdicts. New migrations drop what they created.
10. **Tana and the collector are deployed independently.** *Kept:* the user updates both together (**Out of scope**).

## Behaviour

Terms: the *listing* is the Panda catalog page in Tana (`#/panda`). A *favorite in scope* is a Panda favorite that counts as evidence (behaviour 9). The *catalog in scope* is every Panda catalog gallery that passes the default filter (behaviour 10). The *base rate* of a value is the share of the catalog in scope that carries it. A *held-out favorite* is a favorite in scope scored with weights from the other favorites only (behaviour 26). A *candidate* is the model that the score page shows before it is applied; the *active model* is the applied one. A *calculation* makes a new candidate; a *measurement* says what a threshold would do with a candidate. The score filter is *in force* while a model is active or a score override exists.

Verdicts are gone:

1. A Panda catalog card has no "Interesting" and no "Not for me" control. A card has no control at all besides its link.
2. The view control of the listing offers "All" and "Unseen" only. There are no verdict views: not in the listing, not in Tana's HTTP API, not in the collector.
3. A link or history entry with a `verdict` parameter (`#/panda?verdict=interesting`, `…verdict=not_for_me`) opens the first page of the listing as if `verdict` and `cursor` were absent: the unseen view by default, the view of all galleries with `unseen=false`. The `cursor` is dropped because a position in a verdict view is no position in another view; the unseen view refuses a backward one. Every other parameter of the link applies, including `score_filter`. No link that the page builds carries `verdict`.
4. Tana's HTTP API has no verdict routes, and the collector has none. `GET /api/collector/catalog` and the collector's `GET /api/catalog` answer a request with a `verdict` parameter as they answer it without the parameter.
5. The update deletes all stored verdicts. Nothing exports them. Display records stay, so a gallery that got a verdict stays seen.
6. The score filter hides a gallery by these rules, the first that matches decides: a "never hide" override shows it, an "always hide" override hides it, a score below the threshold of the active model hides it. Nothing else hides a gallery, so a gallery that the user marked "Not for me" is listed again unless one of these rules hides it. The hover details of a card name the deciding rule.
7. A gallery becomes seen only when the user goes to the next page of a listing, or by a catch-up. No card action and no other request makes a gallery seen.
8. A display gives only seen state. It is no evidence for the model, and every display is recorded alike, whatever the score filter state of the listing. The request that records a page carries only gallery IDs.

How the model learns:

9. The evidence is the favorites in scope: each gallery that is a Panda favorite in any stored favorite collection and whose Panda catalog entry passes the default filter (its saved search and its categories). A favorite without a catalog entry is not in scope. A favorite outside the default filter is not in scope. A gallery in several collections counts once. Expunged galleries count like any other, on both sides: the default filter says nothing about them.
10. The catalog in scope is every Panda catalog gallery that passes the default filter, with collected or provisional Panda metadata. A provisional gallery counts with the tags of its feed entry. The favorites in scope are part of it.
11. With F favorites in scope, N galleries in the catalog in scope, and f and n the numbers of those that carry a value: `expected = F × n / N` is the number of favorites that would carry the value if favorites carried it as often as the catalog does, and the smoothed lift is `(f + 10) / (expected + 10)`. The value's weight is the natural logarithm of the smoothed lift.
12. A value gets a weight only when at least 3 favorites in scope carry it and its weight is above 0, which is when it is more frequent in the favorites than in the catalog (`f > expected`). Every other value has no weight. So no calculated weight is negative or 0: a value that is as frequent or less frequent in the favorites than in the catalog adds nothing to a score, is no row of the weights table, and gives a pin no model weight.
13. An unknown uploader or category (an empty value) never gets a weight.
14. The Panda gallery score, pinned weights, score overrides, the score threshold, the author bonus and the favorite authors keep their behaviour. A pin may be negative, and the listing sums it like any weight.
15. The user calculates and applies a model by hand, as today. A visit to the score page calculates a candidate. So do Retry and the request that follows Apply, a saved threshold and Turn off. A calculation changes nothing in the listing. Only "Apply model and threshold" changes the active model. A Panda favorite sync, a change of the default filter, a calculation and a measurement never change the weights of the active model.
16. An override, pin or author bonus change on the score page changes no calculated weight. The request that follows it keeps the candidate that the page shows, with its ID, its calculation time, its weights and their counts, and measures it again with the current pins, overrides and bonus. When that candidate is no longer stored as a candidate (it was applied, or a later calculation replaced it), the request calculates a new one instead.
17. There is no "count Panda favorites" setting. The browser sends nothing that steers what a model learns from: its request names at most the candidate to measure again, and Tana adds the default filter. A model stores no such setting, and the page names none.
18. With no favorite in scope a calculation gives no candidate. With favorites in scope it always gives one, also when no value reaches a weight: such a candidate has no weights, so only pins and the author bonus give a score.

Existing models:

19. The update deletes all stored score models, the active one and every candidate. The score filter is not in force until the user applies a new model, unless a score override exists. Pinned weights, score overrides and the author bonus stay.

Score page (`#/panda/score`):

20. The evidence section shows the number of favorites in scope, the time of the last Panda favorite sync ("never" without one), and, while a model is active, the number of new favorites since that model was calculated.
21. A favorite in scope is new when the collector first observed its gallery as a favorite after the active model was calculated. A favorite that was observed before, removed and favorited again is not new.
22. While a candidate exists and fewer than 50 favorites are in scope, the page warns that weights and the measurement from so little evidence are unreliable. The user can still apply the model.
23. With no favorite in scope the page says that no model is possible, and tells the user to sync the Panda favorites or to widen the default filter. It then shows no weights table, no threshold for a new model and no apply button. The active model, the overrides, the pins and the favorite authors stay usable.
24. The weights table shows for each value its weight, the number of favorites in scope that carry it, the number of galleries of the catalog in scope that carry it, its weight in the active model, and the override and pin controls.
25. For the candidate at the chosen threshold the page shows two shares: the share of the held-out favorites that the threshold hides, and the share of the recent catalog that it hides, beside the share that the listing hides now.
26. A favorite is held out when its weights come from the other favorites only: its score is calculated as if it were not a favorite, with the catalog counts unchanged. A pin counts as in the listing. The tag of a favorite author counts at least the author bonus only when the author stays a favorite author without the held-out favorite, which is when the author has at least 3 favorites. A held-out favorite that matches a "never hide" override is never hidden, and one that matches only an "always hide" override is always hidden, whatever its score.
27. The suggested threshold is the highest threshold, in steps of 0.1, that hides at most 5% of the held-out favorites, counting those that an override always hides. When no threshold does, it is the lowest held-out score; without scored favorites it is 0.
28. The recent catalog sample is the newest 2,000 galleries that the listing shows with the default filter. Each is classed by the first rule that decides it: a "never hide" override, an "always hide" override, else its score under the candidate and under the active model.
29. The page describes the model as learned from the Panda favorites inside the default filter. No text of the page or of the listing mentions verdicts, neutral galleries, "Interesting" or "Not for me".
30. The active model shows when it was calculated and its threshold.

Errors and edges:

31. An invalid default filter, an unreachable collector and a candidate that was replaced before Apply behave as today: the page shows the message and offers a retry or a new calculation.
32. Apply, a threshold change and "Turn off" wait while a calculation or a measurement runs, as today.

## Design

**Data flow.** Unchanged in shape: the browser talks to Tana, each Tana route calls one `collectorapi.Client` method, and the collector's route calls one `catalog.Service` method. Non-test code in `internal/local/` reaches the collector only through `internal/collectorapi`. The collector holds the favorites, the catalog, the model, the pins and the overrides (ADR 0019). Tana holds the default filter and sends it with each preview request.

Signatures, payloads, schema, constants and the stated texts are fixed. Query shapes, private helpers, file names and test sequences are the implementer's. A line that starts with "Hint:" is optional.

### Calculation and measurement (`internal/collector/catalog`)

Exported constants, each with a comment on its purpose:

```go
const (
	PriorStrength         = 10   // pseudo-count added to the favorites that carry a value and to their expected number
	MinSupport            = 3    // favorites in scope a value needs for a weight
	FewFavorites          = 50   // favorites in scope under which a preview warns
	SuggestedFavoriteLoss = 0.05 // largest share of held-out favorites the suggested threshold hides (ticket 06)
)
```

`NeutralWeight`, `FewVerdicts`, `SuggestedInterestingLoss`, the evidence classes, the log-odds calculation and the fold measurement go (ticket 04).

- **Scope (ticket 04).** The default filter of the request, which is its search and its categories, decides both sides, and expunged galleries are in scope. The favorites in scope are the distinct galleries of every row of `favorites` (all collections, accounts and hosts) whose `catalog_galleries` row passes. The catalog in scope is every `catalog_galleries` row that passes. So the favorites in scope are a subset of the catalog in scope, and every expected count is defined.
- **Weight (ticket 04).** `weight = ln((f + PriorStrength) / (F*n/N + PriorStrength))`, kept only when `f >= MinSupport` and `weight > 0`. A tag counts by the catalog's tag rows, an uploader and a category by the gallery's own values, an empty uploader or category never.
- **Why this smoothing.** With F = 1,000 and N = 1,000,000:

  | Value | f | n | expected | weight |
  |---|---|---|---|---|
  | artist, 3 favorites of 5 galleries | 3 | 5 | 0.005 | 0.26 |
  | artist, 40 favorites of 60 galleries | 40 | 60 | 0.06 | 1.60 |
  | tag on 30% of the catalog, 60% of the favorites | 600 | 300,000 | 300 | 0.68 |
  | tag on 90% of the catalog, 95% of the favorites | 950 | 900,000 | 900 | 0.05 |
  | tag on 30% of the catalog, 20% of the favorites | 200 | 300,000 | 300 | none |

  Smoothing the favorite frequency toward the base rate instead (`(f + 10·n/N) / ((F + 10)·n/N)`) gives the first row a weight of 6.39, because a prior scaled by a tiny base rate damps nothing. Rare artist tags would then top the table. The weight stays a logarithm, because pins, the author bonus and thresholds already sit on that scale.
- **Catalog counts, the risky part (ticket 04).** Three constraints hold, and the query layout is the implementer's: the favorites in scope may be read with their values into Go, but catalog galleries are never read into Go; the catalog is counted per value only for the values that at least `MinSupport` favorites in scope carry; and the whole preview stays one transaction under its lock. A common tag is among those values as soon as a few favorites exist, so a calculation reads a large part of `catalog_gallery_tags`. Its time is not measured (**Assumptions**).
- **Candidate (ticket 04).** Stored as today in `panda_score_models` and `panda_score_weights`, with what it was calculated from: each weight row keeps f and n, the model row keeps F and N. `Candidate.Weights` lists the 500 strongest, each with its two counts and its weight in the active model; `WeightCount` is the number of weights. No row stores a favorites setting.
- **No model (ticket 04).** `NoModel` is `collectorapi.ScoreNoFavorites` when a calculation finds no favorite in scope, and empty otherwise. `ScoreNoVerdicts`, `ScoreNoNotForMe` and `ScoreNoAcceptable` go.
- **Evidence numbers (ticket 04; `NewFavorites` ticket 07).** `Favorites` is the number of favorites in scope at the time of the request. `FavoritesSyncedAt` stays the latest `favorite_categories.synced_at`. `NewFavorites` is the number of favorites in scope whose `favorite_observations.observed_at` is later than the active model's `calculated_at`; nil without an active model. A favorite without an observation row is not new. `FewFavorites` is `Favorites < FewFavorites`.
- **Recent sample (ticket 04, pinned again in ticket 06).** It keeps the listing's predicate (default filter, expunged left out) and `RecentSampleSize`, and classes by the two override rules and the score only. It no longer reads verdicts, and it stays real in every ticket.
- **Held-out favorites (ticket 06).** For a favorite in scope and each of its values, the held-out weight is the weight of behaviour 11 and 12 with `f - 1` and `F - 1` and the catalog counts unchanged. A value without a weight in the candidate has none in a held-out score either (`f ≤ F·n/N` gives `f - 1 ≤ (F - 1)·n/N`), so the candidate's stored counts and totals are all a held-out score needs. On top, a pin replaces the held-out weight as in the listing, and the tag of a favorite author weighs at least the author bonus only when its stored `favorite_authors.favorites` is at least 3. `Candidate.HeldOut` counts every favorite in scope: those that a "never hide" override decides, those that only an "always hide" override decides, and the scores of the rest, in the histogram window of today.
- **Suggested threshold (ticket 06).** The rule of behaviour 27 over `Candidate.HeldOut`.
- **Measuring again (ticket 08).** A preview request with a `CandidateID` that names the stored candidate keeps that candidate: it calculates no weight, counts nothing in the catalog and stores no model. It answers the candidate's ID, calculation time, weights and counts from the stored rows. Everything else is read at the time of the request, with the request's default filter: the evidence numbers, the held-out favorites (the favorites in scope now, each with the stored counts), the suggested threshold, the recent sample, the pins with their model weights, the favorite authors, the overrides and the active model. A measurement never removes the candidate, also when no favorite is in scope any more. A `CandidateID` of 0, or one that names no stored candidate, calculates as a request without it; that is no error.
- **Locking.** A calculation and a measurement take the preview's `SHARE ROW EXCLUSIVE` lock on `panda_score_models` and run in one transaction, as the preview does today. `ApplyScoreModel`, `SetScoreThreshold` and `RemoveScoreModel` are unchanged.
- **Score filter (ticket 02).** The hiding rules of behaviour 6, in that order, with no read of verdicts.

### `internal/collectorapi`

Final shapes (`catalog_score.go`, `catalog.go`, `catalog_displays.go`); the ticket that changes each part is named:

```go
// ScorePreviewOptions asks for a candidate score model from the Panda
// favorites inside the default filter. (ticket 03)
type ScorePreviewOptions struct {
	DefaultQuery      string   `json:"default_query"`
	DefaultCategories []string `json:"default_categories"`
	// CandidateID names the stored candidate to measure again; 0 calculates. (ticket 08)
	CandidateID int64 `json:"candidate_id,omitempty"`
}

type ScorePreview struct { // no Favorites field (ticket 03)
	Evidence  ScoreEvidence   `json:"evidence"`
	Candidate *ScoreCandidate `json:"candidate"`
	NoModel   string          `json:"no_model,omitempty"`
	Active    *ScoreActive    `json:"active"`
	Overrides []ScoreOverride `json:"overrides"`
	Pins      []ScorePin      `json:"pins"`
	Recent    ScoreRecent     `json:"recent"`
	// AuthorBonus, FavoriteAuthorCount, FavoriteAuthors: unchanged
}

const ScoreNoFavorites = "no_favorites" // ticket 04

type ScoreEvidence struct { // ticket 04
	Favorites         int        `json:"favorites"`
	FavoritesSyncedAt *time.Time `json:"favorites_synced_at"`
	NewFavorites      *int       `json:"new_favorites"` // nil until ticket 07 fills it
	FewFavorites      bool       `json:"few_favorites"`
}

type ScoreCandidate struct { // no Favorites field (ticket 03)
	ID                       int64         `json:"id"`
	CalculatedAt             time.Time     `json:"calculated_at"`
	WeightCount              int           `json:"weight_count"`
	Weights                  []ScoreWeight `json:"weights"`
	SuggestedThresholdTenths int           `json:"suggested_threshold_tenths"` // 0 until ticket 06
	HeldOut                  ScoreHeldOut  `json:"held_out"`                   // zero until ticket 06
}

// ScoreHeldOut counts the held-out favorites: those an override always
// hides or never hides, and the scores of the rest. A threshold of t tenths
// hides AlwaysHidden plus the buckets below t. (shape: ticket 04)
type ScoreHeldOut struct {
	Total        int            `json:"total"`
	AlwaysHidden int            `json:"always_hidden"`
	NeverHidden  int            `json:"never_hidden"`
	Scores       ScoreHistogram `json:"scores"`
}

type ScoreWeight struct { // ticket 04
	Kind         string   `json:"kind"`
	Label        string   `json:"label"`
	Weight       float64  `json:"weight"`
	Favorites    int      `json:"favorites"` // favorites in scope that carry the value
	Catalog      int      `json:"catalog"`   // galleries of the catalog in scope that carry it
	ActiveWeight *float64 `json:"active_weight"`
}

type ScoreActive struct { // no Favorites field (ticket 03)
	CalculatedAt    time.Time `json:"calculated_at"`
	ThresholdTenths int       `json:"threshold_tenths"`
}

type ScoreRecent struct { // no Interesting, no NotForMe (ticket 04)
	Total        int             `json:"total"`
	NeverHidden  int             `json:"never_hidden"`
	AlwaysHidden int             `json:"always_hidden"`
	Candidate    *ScoreHistogram `json:"candidate"`
	Active       *ScoreHistogram `json:"active"`
}
```

`ScoreClassEffect` and the three-class `ScoreHeldOut` go in ticket 04. Between tickets 04 and 06 a candidate carries a zero `SuggestedThresholdTenths` and a `HeldOut` with zero counts and an empty `Scores.Counts` (`[]`, never null).

Removed: `Client.SetCatalogVerdict`, `Client.RemoveCatalogVerdict` (ticket 01); `VerdictInteresting`, `VerdictNotForMe`, `ValidVerdict`, `CatalogOptions.Verdict`, `CatalogItem.Verdict`, the `verdict` parameter of `Client.Catalog` (ticket 09). Changed: `func (c *Client) RecordCatalogDisplays(ctx context.Context, ids []int64) error` (ticket 10).

### Routes

| Route | Change | Ticket |
|---|---|---|
| Tana `PUT`, `DELETE /api/collector/catalog/verdicts/{id}` | removed | 01 |
| Tana `GET /api/collector/catalog` | no longer forwards `verdict` | 01 |
| Tana `POST /api/collector/catalog/score/preview` | request body is `{}` | 03 |
| Collector `POST /api/catalog/score/preview` | request body is `{"default_query": "...", "default_categories": [...]}` | 03 |
| Tana `POST /api/collector/catalog/score/preview` | request body is `{}` or `{"candidate_id": 7}`; Tana forwards the ID with the default filter | 08 |
| Collector `POST /api/catalog/score/preview` | request body may also carry `"candidate_id": 7` | 08 |
| Collector `PUT`, `DELETE /api/catalog/verdicts/{id}` | removed | 09 |
| Collector `GET /api/catalog` | no longer reads `verdict` | 09 |
| Tana `POST /api/collector/catalog/displays`, collector `POST /api/catalog/displays` | request body is `{"gallery_ids": [...]}` | 10 |

Routes stay in the two `httpapi/` packages. Both sides decode request bodies strictly, so a field that a body lost (`favorites`, `score_filter`) is an unknown field from then on; that follows from `server.DecodeJSON` and gets no test of its own. `catalog.Service` loses `SetVerdict`, `RemoveVerdict` and `ErrInvalidVerdict` (ticket 09) and gets `func (s *Service) RecordDisplays(ctx context.Context, ids []int64) error` (ticket 10). The error code `invalid_verdict` goes.

### Collector schema (`internal/collector/storage/migrations`)

Each ticket adds one migration with the next free number and a comment on its purpose. Numbers below assume the ticket order. No applied migration is edited.

- `0011` (ticket 03): drops `panda_score_models.favorites`.
- `0012` (ticket 04): deletes every row of `panda_score_models` (the weights follow by their foreign key); drops `panda_score_weights.interesting`, `not_for_me` and `neutral`; adds `panda_score_weights.favorites INTEGER NOT NULL` and `catalog INTEGER NOT NULL` (f and n of the weight's value), and `panda_score_models.favorites_in_scope INTEGER NOT NULL` and `catalog_in_scope INTEGER NOT NULL` (F and N of its calculation). The tables are empty when the columns are added, so they need no default.
- `0013` (ticket 07): adds `favorite_observations.observed_at TIMESTAMPTZ NOT NULL DEFAULT now()`. Existing observations get the migration's time. Every model that exists after the update is calculated later, because `0012` deleted the earlier ones.
- `0014` (ticket 09): drops `panda_gallery_verdicts`.
- `0015` (ticket 10): drops `panda_gallery_displays.evidence`.

A migration test migrates to the state before its migration, fills rows, and then applies every migration, also the later ones. So each such test asserts only what stays true after the whole update.

`ObserveFavorite` and `SeedFavoriteObservations` insert only `gallery_id`, so the default fills `observed_at` and the `favorites` package needs no change. Run `go generate ./internal/collector/storage/` after each migration and commit any `dbgen` change.

### Web (`web/src`)

- **Listing (ticket 01).** `navigation.ts`: `PandaView` is `'all' | 'unseen'`; `isVerdictView` goes; the route reads `verdict` only to drop the `cursor` of such a link; `pandaListingLink` never writes `verdict`, and `score_filter` is read from every Panda link. `panda/api.ts`: `PandaVerdict`, `verdictLabels`, `savePandaVerdict` and `PandaGallery.verdict` go; `listPandaCatalog` sends no `verdict`. `PandaCard`: no `onVerdict`, no verdict controls, and the deciding rule comes from `score.rule` alone. `PandaCatalog`: two view items, no verdict save, no verdict alert, no verdict empty state. `GalleryBox.actions`, `.gallery-box-cell`, `.gallery-box-actions` and `.panda-verdict-error*` have no other user and go.
- **Score request (tickets 03 and 08).** Ticket 03: `previewPandaScore(signal)` posts the body `{}`; the page has no favorites switch; the active model reads "Calculated <time>, threshold <t>." Ticket 08: `previewPandaScore(candidateID: number | null, signal)` posts `{"candidate_id": <id>}` or `{}`. The page sends the ID of the candidate it shows with the request that follows an override change, a pin change or an author bonus change, and `{}` when it shows no candidate. The first request of a visit, Retry, and the request that follows Apply, a saved threshold, Turn off or a failed Apply send `{}`.
- **Score types (`panda/score.ts`, ticket 05).** Mirror the Go shapes: `ScoreEvidence { favorites, favorites_synced_at, new_favorites, few_favorites }`, `ScoreWeight { kind, label, weight, favorites, catalog, active_weight }`, `NoScoreModel = 'no_favorites'`, `ScoreCandidate.held_out: HeldOut` with `{ total, always_hidden, never_hidden, scores }`, `RecentCatalog` without `interesting` and `not_for_me`. The share of the held-out favorites that a threshold hides is `(always_hidden + buckets below the threshold) / total`; the share of the recent sample is `(recent.always_hidden + buckets below the threshold) / recent.total`; both are 0 for a total of 0.
- **Score page texts (ticket 05).** Fixed, so tests and tickets agree:
  - Intro: "Calculate a model that scores catalog galleries by their tags, uploader and category, learned from your Panda favorites inside the default filter."
  - Evidence labels: "Favorites in scope", "Last Panda favorite sync", "New favorites since the active model was calculated".
  - Warning: "Fewer than 50 favorites are in scope. Weights and the measurement from so little evidence are unreliable."
  - No model: "No model is possible: no Panda favorite is in scope. Sync your Panda favorites, or widen the default filter."
  - Weights table headers: "Tag, uploader or category", "Weight", "Favorites", "Catalog", "Active weight", "Override", "Pin".
  - Measurement: "Hides <x> % of the held-out favorites. A favorite is held out when its weights come from the other favorites only." and "Hides <y> % of the newest <n> catalog galleries (now: <z> %)".
  - Suggested: "Suggested: <t> (hides at most 5 % of the held-out favorites)".
- **Display record (ticket 10).** `recordPandaDisplays(galleryIDs: number[])` sends `{"gallery_ids": [...]}` in every score filter state.

### ADR 0019 (ticket 09)

The brief orders this correction, so it is no contradiction to resolve. The ADR then describes only the current design (`AGENTS.md`): the collector keeps the seen state, the score model with its threshold, the overrides, the pins, the author bonus and the favorite authors. The verdict paragraph goes. The model paragraph says that the model is calculated from the Panda favorites and the catalog that the collector holds, and that Tana sends its default filter with each preview request because the filter limits both sides. The display sentence no longer calls a display evidence.

### Test seams

- **Stored model and stored candidate** (from ticket 02): what the listing does with an active model (hiding, scores, strongest weights, summary tags, pins, the author bonus, tag weights) and what apply, a threshold change and turn-off do with models (the lifecycle) are tested in `internal/collector/catalog` and `internal/local/httpapi` against rows that the test stores directly in `panda_score_models` and `panda_score_weights`. A stored model may hold negative weights: the listing sums stored weights of either sign, as it does pins. The code that inserts these rows follows the schema: it stops inserting the columns that tickets 03 and 04 drop, and from ticket 04 on it gives the new count columns any numbers.
- **Calculation and measurement**: `catalog.Service.PreviewScoreModel` in `internal/collector/catalog`, with galleries from the existing retain and feed projection test helpers and rows in `favorite_categories`, `favorites`, `favorite_observations` and `favorite_authors`. The weight formula also has a unit test of its pure function.
- **Migrations**: `internal/collector/storage/storage_test.go` style, with `pgtest.Open` on the migrations before the new one and `pgtest.Reopen` on all.
- **Routes**: `internal/collector/httpapi/catalog_test.go` for validation, `internal/local/httpapi/catalog_test.go` through the real collector handler.
- **Web**: Vitest with a mocked `fetch`, in `web/src/panda/*.test.tsx`, `web/src/panda/score.test.ts`, `web/src/navigation.test.tsx` and `web/src/App.test.tsx`.

Tests of removed behaviour are deleted, not ported, and no new test pins that something is gone (`AGENTS.md`: "Cover retained features only"). This holds for verdict storage, verdict views, the verdict routes, display evidence, the favorites switch, the removed request fields and the three evidence classes. A ticket states such a removal as a criterion that starts with "Removal:", which review checks by reading the diff or by the named search. The two exceptions are behaviour that the brief keeps for old links: a link with `verdict` opens the normal view, and both catalog routes ignore a `verdict` parameter.

## Assumptions

- **Smoothed lift is `ln((f + 10) / (expected + 10))`.** Rejected: smoothing the favorite frequency toward the base rate, which leaves rare values at weights near `ln(N/F)` (table in **Design**). Rejected: the raw ratio without a logarithm, because scores add weights and pins, the bonus and thresholds are on a log scale.
- **A value that is not more frequent in the favorites has no weight**, not a weight of 0. Rejected: storing 0, which would rank such tags before unweighted ones on cards, fill the weights table with zeros and grow the model.
- **`MinSupport` stays 3 and `PriorStrength` stays 10.** Rejected: dropping the support limit, which adds thousands of tiny weights for values that one or two favorites carry.
- **Expunged galleries count on both sides.** The default filter is a search and categories and says nothing about expunged galleries, and the brief names only a sync or a wider default filter as the remedy for "no favorite in scope". Rejected: the listing's predicate, which would drop expunged favorites without telling the user.
- **Favorites of every stored account and host count**, as the favorite authors do. Rejected: only the configured account.
- **A candidate exists whenever a favorite is in scope**, also without any weight. Rejected: a second "no model" reason, which the brief does not name.
- **A favorite is new by first observation**, read from a new column `favorite_observations.observed_at`. This follows ADR 0010, where "new" means first observed, whatever Panda's timestamp. Rejected: `favorites.added_at`, which misses a backlog that a later sync collects; a stored count to compare with, which hides one added and one removed favorite; a column on `favorites`, which a category move would reset.
- **Held-out means leave one out**, with the catalog counts unchanged. Rejected: the five folds of today, which need a constant and say less directly "from the other favorites only".
- **The author bonus is held out too.** In a held-out score the tag of a favorite author gets the bonus only when the author has at least 3 favorites, so that it is a favorite author without the held-out one. Rejected: the bonus as in the listing. An author with exactly 2 favorites would give each of the two the bonus only because it is itself a favorite, which lifts the held-out scores and the suggested threshold with them. The count is the stored `favorite_authors.favorites`, which is as old as the last completed favorite sync and counts favorites outside the default filter too, like the favorite authors themselves.
- **The time of a calculation is not measured.** The real collector could not be reached while this plan was written: Tana's `GET /api/collector/inventory/status` answered `collector_unavailable` twice. What is known: the unseen-view spec plans for "millions of catalog rows"; `catalog_gallery_tags` holds one row per tag of each gallery, so several times that; `catalog.go` already shapes its page queries so that they never enumerate the galleries of a common tag; and the request has 30 s on three hops (the collector's write timeout, Tana's collector client, Tana's write timeout). A tag on a large share of the catalog is carried by 3 favorites as soon as a few favorites exist, so each calculation reads a large part of `catalog_gallery_tags`. Whether that fits 30 s on the real collector is unknown. Decided now, not after the build: the count runs once per visit and never for a pin, override or bonus change (next assumption), and the time of that one calculation is an accepted risk that the first Manual check measures. If it fails, the remedy is a base rate that is stored per default filter or taken from a sample; both are out of scope. Rejected: a sampled base rate now, which makes the catalog count of a rare value an estimate and differs from the brief's "the Panda catalog under the same default filter".
- **An override, pin or bonus change measures the stored candidate again** (behaviour 16), and the candidate stores its counts for that. Rejected: recounting on every preview, which puts the catalog count and its lock behind every pin and override edit, now the only way to express dislike. Rejected: a table of catalog counts per default filter, which needs its own invalidation. Rejected: an error for a `candidate_id` that names no stored candidate; a new calculation is what the page would ask for next.
- **A measurement uses the candidate as calculated and everything else as it is now.** After a favorite sync or a change of the default filter between the calculation and a measurement, the weights and counts are still those of the calculation, while the evidence numbers, the held-out favorites and the recent sample follow the current state. A favorite that was not counted is then held out like the others. The next visit calculates anew. Rejected: storing the default filter and the sync time with the candidate to detect the change, two more columns for a case that needs a second tab or a sync during the visit.
- **A saved threshold and Turn off calculate again**, like Apply, although they change no weight. They are rare, and "a change of the model starts the page again" is the rule the page has today.
- **Several migrations instead of one.** The brief says "a migration"; one per ticket keeps each ticket green alone, and the user gets the same result from one update.
- **A link with `verdict` opens the first page of the default view.** Its other parameters apply, its `cursor` does not, and any value of `verdict` counts. Rejected: keeping the cursor, which shows "Invalid catalog position" for a backward one and a page from the middle of the catalog for a forward one. Rejected: redirecting to the view of all galleries, which would ignore the default view.
- **The whole score page changes in one ticket** (05), and the calculation and the page are two adjacent tickets (**Fit** 3). Rejected: one ticket for both, which does not fit one implementer context; an evidence ticket and a threshold ticket for the page, which would leave the threshold block throwing between them.
- **The score page offers no link beside the "no model" text**, only the text.
- **Terms missing from `CONTEXT.md`:** "favorite in scope", "catalog in scope", "base rate", "held-out favorite", "calculation", "measurement". The spec defines them under **Behaviour**; the glossary is not changed here.

## Out of scope

- An export of the verdicts, and any way to restore them.
- A list of values that are less frequent in the favorites than in the catalog (candidates for a negative pin).
- The number of galleries of the catalog in scope on the score page.
- A stored or sampled base rate that makes the first calculation of a visit faster, and a score page that reads its state without a preview.
- Automatic recalculation after a favorite sync or a change of the default filter, and a "calculate again" control beside a new visit.
- Compatibility between a new Tana and an old collector or the reverse: the user updates both together.
- Changes to the favorite authors, the author bonus in the listing, pins and overrides.
- Glossary changes in `CONTEXT.md`.

## Manual checks

- **Calculation time (do this first).** Open `#/panda/score` against the real collector with the real default filter and time the first answer. Good: a few seconds, far inside the 30 s limit. There is no constant to tune. A calculation near or past 30 s means that the catalog counts read too much: the follow-up is a base rate stored per default filter or taken from a sample.
- **Measurement time.** On the same page, pin a weight, add an override and change the author bonus. Good: each answer comes clearly faster than the first one, the weights table and its counts do not change, and the two shares and the suggested threshold follow the change.
- **Waiting changes.** While a calculation runs, save a threshold of the active model. Good: it saves as soon as the calculation ends, and the listing stays usable meanwhile.
- **Weights.** Read the top of the weights table against the favorites. Good: the strongest weights are values the user recognises as taste (authors with many favorites, liked tags), a value that 3 favorites carry stays near 0.26, and no value that is on nearly every gallery ranks high. The constants to tune are `PriorStrength` (higher damps small counts more) and `MinSupport`.
- **Threshold.** Move the candidate threshold from the suggested value upward. Good: the share of hidden held-out favorites grows slowly while the share of the hidden recent catalog grows fast. A suggested threshold of 0.0 that hides nothing means that more than 5% of the favorites share no weighted value with the others; the constant to tune is `SuggestedFavoriteLoss`.
- **The update.** Start the collector on a copy of the real database. Good: the migrations run in seconds, the listing shows no score filter control unless an override exists, galleries that were "Not for me" are back, galleries that had a verdict are still seen, and the score page shows a plausible number of favorites in scope with 0 new favorites after the first apply.
- **Cards and toolbar.** Look at the listing on a wide and a narrow window. Good: cards have no gap where the controls were, the hover look of a card is as before, and the view control with two items leaves the toolbar on one row.
- **Weights table width.** Good: the two count columns fit beside the pin controls, and the table scrolls sideways before a cell breaks.

The plan picks no timing constant: no delay, no debounce and no polling interval.
