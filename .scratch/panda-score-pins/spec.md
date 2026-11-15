# Pinned weights and favorite authors for the Panda score

## Problem

The Panda score model calculates every weight from the user's evidence, and the user cannot correct one. A tag that the model weighs wrongly, or a tag with too little evidence for a weight, stays as it is until more verdicts change it. The only tools by hand are the score overrides, and an override decides alone: "always hide" hides a gallery whatever else it carries. Galleries by artists whose works the user keeps as Panda favorites get no help either. An artist with two or three favorites has little evidence, so the model gives the artist tag a small weight or none, and a new gallery by that artist falls below the score threshold because of its other tags.

## Solution

The user pins the weight of a tag, an uploader or a Panda gallery category to a number. The pinned weight replaces the calculated weight in the Panda gallery score, also for a value the model has no weight for, and 0 ignores the value. The score stays a sum, so a pin shifts a gallery and never decides alone. A pin works in the Panda listing as soon as it is saved, and it stays through every calculation and model change until the user removes it. The user pins on the evaluation page (on each row of the weights table, with a form, and in a list of all pins) and in the tags section of a local gallery's detail view.

The user also sets one author bonus on the evaluation page. From then on the tag of each favorite author, an artist with at least two Panda favorites, weighs at least the bonus. The set of favorite authors follows the Panda favorites with each completed Panda favorite sync. Every number of the evaluation page counts the pins and the bonus. The cards of the Panda listing mark pinned tags and the tags of favorite authors, and their hover details name the source of each weight.

## Fit

Fits after two small prefactors (tickets 01 and 02). Parts of the design that exist only because of how the code is built today:

1. **`scoreSum`, `markWeights` (`score_filter.go`) and `summarizeTags` (`catalog.go`) each read `panda_score_weights` with their own SQL.** A pin or the bonus added to one and missed in another gives a card that contradicts the filter. *Prefactor (ticket 01):* one private builder gives the SQL expression for the weight of a tag, an uploader or a category under a model, and the three take every weight from it. No exported name changes, and no test is edited. Tickets 04 and 07 then change the rule in that one place.
2. **The preview's histograms are dense: `ScoreHistogram.Counts` has one count for every tenth between the lowest and the highest score** (`scoreHistogram` in `score_recent.go`), and `scoreFilter.recent` casts each score's tenths to `integer`. Calculated weights are small log odds, so this holds today. A pin has no such size. One recent gallery with a tag pinned to -100000 gives a histogram of about a million counts, five such histograms pass the 4 MiB that `catalogJSONRequest` reads, a score beyond about ±2.1e8 fails the cast, and the evaluation page, the only place that lists the pins, could no longer load. *Prefactor (ticket 02):* every histogram counts inside a fixed window of scores from -1000 to 1000, and a score beyond it counts at the window's edge. No exported name changes, and no test is edited. *Cost kept:* for a threshold outside the window the page's numbers are not exact (behaviour 35). Rejected: sparse histograms (pairs of bucket and count). They keep every threshold exact, but they change `collectorapi.ScoreHistogram`, `score.ts`, and the fixtures and assertions of four existing test files, which is a large prefactor for a threshold nobody sets.
3. **PostgreSQL raises an error when a sum or a product of floats leaves the float range**, and the listing calculates `floor(sum * 10)` for each candidate. One pin near the largest float would fail every catalog page that meets its value, and the preview with it. *Kept:* a pin and the author bonus have a technical bound of 1e300 in magnitude (`maxScoreWeight`). It is no limit a user meets (behaviour 5).
4. **`heldOutTenths` scores the evidence in Go** with weights calculated per fold, which no table holds, while the listing and `scoreFilter.recent` score in SQL. *Kept:* the weight rule exists once in SQL and once in Go (**Design → The weight rule**), and the tickets test both. Rejected: storing the fold weights so that SQL scores the held-out galleries, which changes how a preview is calculated and stored.
5. **A model's weights live in `panda_score_weights` under their model**, and `PreviewScoreModel` and `ApplyScoreModel` delete models. *Fits:* pins and the bonus get tables of their own that reference no model.
6. **`belowThreshold` is `false` without an active model, and `summarizeTags` attaches weights only with one.** *Fits:* the brief gives pins no effect without a model, so both stay, and a card shows a pin or a favorite author only while a model is active.
7. **`SetScoreOverride` holds the value rules inline** (trim, `namespace:value`, the folded tag match, the category list). A pin has the same rules. *Kept inside ticket 03:* it moves them into one private helper that both use. It is a few lines, and the override tests pass unedited.
8. **The preview is the evaluation page's only read, it takes seconds, and `runChange` locks the page until it answers.** Several pins in a row would each wait for a calculation. *Kept as an addition (tickets 03 and 11):* the pin route answers with the stored pin and takes no lock on `panda_score_models`. The page puts the answer into its pin list at once and starts the calculation in the background, where a newer one replaces it. Rejected: a cheap read of the page's state beside the calculation. It would split `collectorapi.ScorePreview`, both HTTP layers and most of `PandaScorePage.test.tsx`.
9. **`favorites` holds no tags, and `scoreFilter.hidden` tests each candidate of the ordered catalog scan.** Counting favorites per candidate, or per catalog page, is too slow. *Fits:* the collector keeps the set of favorite authors in a table that the score expression probes by key. `favorites.store.savePage` refreshes it in the transaction that completes a sync, with queries of the `favorites` package. That package already reads and writes across tables in that transaction (`downloads.EnqueueInTransaction`), and `savePage` transactions run one after another because each locks the download settings row first.
10. **A favorite's catalog entry can arrive after the sync that found it**, because the sync only queues its metadata. *Kept:* such a favorite counts from the next completed sync on (behaviour 16). The brief ties the set to completed syncs.
11. **`List` and the preview run at READ COMMITTED and read the pins in more than one statement.** A pin saved between two statements can give one page whose marks are newer than its hidden state. *Kept:* the next load is right, as for an apply between two statements today.
12. **`GalleryBox` is one link with a tooltip that takes no input.** *Fits:* the cards get marks only (brief).
13. **Each tag of `GalleryDetail.tsx` is a toggle for "Search selected tags", and the view works without the collector.** *Fits:* the weight and the pin editor sit beside the toggle, never inside it, and come from one optional request. Without a valid answer the view renders what it renders today.
14. **Local tags can come from `galleryinfo` files and be unknown to `catalog_tags`.** *Fits:* the tag weight lookup says for each tag whether the Panda catalog has it, so the view shows the reason before the user tries to pin.
15. **Collector migrations must be numbered without a gap** (`pgdb.migrate`). *Kept as blocking edges:* 0008 (ticket 03), 0009 (ticket 06), 0010 (ticket 07).
16. **Tana and the collector are deployed independently.** *Kept:* the user updates both together. Against an older collector the detail view looks as today (the lookup fails), and the evaluation page needs the new preview fields.
17. **One existing assertion pins that the detail view of a gallery without a Panda candidate sends one request** (`GalleryDetail.test.tsx`), and the stubs of the detail view's tests answer every path with the same JSON. *Changed in ticket 14:* the assertion says instead that no lookup request is sent, and the view takes only a valid tag weights answer (**Design → Detail view**). No other existing assertion changes in any ticket.

## Behaviour

Terms: the *listing* is the Panda catalog page in Tana (`#/panda`) in any view, search or score filter state. The *evaluation page* is `#/panda/score`. The *detail view* is the page of one local gallery (`#/galleries/<id>`). A *pin* is a pinned weight. The *calculated weight* of a tag, uploader or category is its weight in a Panda score model. Its *effective weight* is the weight it has in the Panda gallery score: the pin, the author bonus or the calculated weight, by behaviours 2, 19 and 20. The *source* of an effective weight is the pin, the author bonus, or the model. The *strip* is the row of tags below a card's cover. A *candidate* is the model the evaluation page shows before it is applied, and the *active model* is the applied one. The score filter is *in force* when a model is active or a score override exists.

Pinned weights:

1. A pin is a number that the user sets for one tag, uploader or Panda gallery category. A value has at most one pin: saving a pin for a value that has one replaces its number.
2. In the Panda gallery score, the pin replaces the calculated weight of its value. The calculation itself does not change: a model's weights are calculated as before, with or without pins.
3. A pin also counts for a value that has no calculated weight, for example a tag that fewer than 3 evidence galleries carry.
4. The score stays the plain sum of the effective weights of the gallery's tags, its uploader and its category. A pin never hides or keeps a gallery alone: positive weights can outweigh a negative pin, and negative weights a positive one.
5. Every number is permitted, positive or negative. The only bound is technical: a number whose magnitude is above 1e300 is refused, because a sum of such numbers leaves the number range of the database. A pin of 0 ignores the value: it adds nothing to the score, whatever its calculated weight is.
6. A pin acts on the listing when it is saved: the next catalog page uses it, and so does the removal of a pin. No calculation and no "Apply" is needed.
7. A pin belongs to no model. It stays across calculations, "Apply model and threshold", a threshold change and "Turn off", and it applies to every model that becomes active. Only the user removes it.
8. The hiding rules keep their order: the verdict, a "never hide" override, an "always hide" override, then a score below the threshold. A pin changes only the score.
9. Without an active model, pins have no effect: no gallery is hidden by its score, and pins alone do not put the score filter in force. The user can still set and remove pins.
10. A pin has the value rules of a score override. A tag is named as `namespace:value` and must be a tag of the Panda catalog, matched without regard to case and with `_` for a space. An uploader is named exactly as Panda writes it. A category is one of the Panda gallery categories. An invalid value and an unknown tag are refused, each with the message of the override form.
11. A value can have a pin and a score override. Both are kept, and the override decides before the score.

Favorite authors:

12. A favorite author is an `artist` tag that at least 2 Panda favorites carry. A gallery that is in several Panda favorite collections, also of several Panda accounts, counts one time. The minimum of 2 is fixed, and no other namespace gives a favorite author, also not `group`.
13. A favorite counts for its authors only when the Panda catalog has an entry with tags for its gallery, from collected or provisional Panda metadata.
14. The set of favorite authors changes when a Panda favorite sync completes, and at no other time: the collector then reads it from the stored favorites and the catalog's tags. No calculation and no "Apply" is needed.
15. A completed sync that is not a full re-sync adds favorite authors and updates their favorite counts, and it removes none. A completed Panda favorite full re-sync also removes each author that no longer has 2 favorites. A sync that is running or has failed changes nothing in the set.
16. A favorite whose catalog entry arrives after the sync counts from the next completed sync on.
17. The update that brings this feature fills the set from the favorites the collector holds at that time, without a sync.
18. The author bonus is one number above 0, with the technical bound of behaviour 5, that the user sets on the evaluation page. It is off until the user sets it, and the user can turn it off again.
19. While the bonus is on, the tag of each favorite author weighs at least the bonus: its effective weight is the larger of its calculated weight and the bonus, and the bonus when it has no calculated weight.
20. A pin on a favorite author's tag wins: the effective weight is the pin.
21. The bonus applies under every model, whether or not that model counts Panda favorites as "Interesting".
22. A new bonus, and turning it off, act on the listing when saved, like a pin. Without an active model the bonus has no effect.

Listing and cards:

23. The listing scores each gallery with the effective weights in every view and every score filter state. The score that the hover details show is that sum.
24. With an active model, the strip ranks the tags by the absolute value of their effective weight and colours them by it, with today's bounds (green from 0.2, red from -0.2). A tag with a pin of 0 is gray and ranks before the tags without a weight.
25. A tag whose effective weight is its pin has a pin mark before its value. Assistive technology reads "Pinned: " before the value.
26. With an active model, the tag of a favorite author has a star mark before its value, read as "Favorite author: ". It has the star whether the bonus is on or off, and whether the bonus or the model gives its weight. A favorite author's tag with a pin shows the pin mark and no star.
27. Tags with a score override stay first, "never hide" before "always hide", with the override's mark and colour. An override tag shows only the override mark, also when it has a pin or is a favorite author's tag.
28. The hover details list the gallery's strongest effective weights (up to five, among its tags, uploader and category) and name the source of each that is not calculated: "(pinned)", or "(author bonus)" where the bonus gives the weight.
29. A card offers no pin action. A loaded page keeps its cards when a pin or the bonus changes elsewhere, and the next load shows the change.
30. Without an active model, a card shows no weights, no pin mark and no star, as today.

Evaluation page:

31. Each row of the weights table shows the calculated weight and the pin: a number field holds the pin's number and is empty without a pin. "Pin" saves the field's number, "Ignore" saves a pin of 0, and "Remove pin", on a pinned row, removes the pin.
32. A section "Pinned weights" lists all pins, also those of values that are not in the table, each with its number and "Remove". Without pins it says so.
33. A form "Add pin" pins a tag, uploader or category that is not in the table. It has the value rules and the messages of the override form, also for an unknown tag.
34. A pin change is saved at once and shows at once in the row and in the list. The page then calculates again in the background, and a calculation that is still running is replaced. The pin controls stay usable during a calculation, so several pins in a row do not wait for one another. All other controls wait for the calculation, as today. The threshold fields keep their values.
35. Every number of the page about the candidate or the listing counts the pins and the author bonus: the shares of "Not for me", "Interesting" and neutral evidence that a threshold hides, the two error rates, the suggested threshold, the share of recent catalog galleries that the candidate and the active model hide at a threshold, and the share that the listing hides now. The page loads with pins and a bonus of every permitted size. Its numbers count each score inside a window from -1000 to 1000, and a score beyond the window at the window's edge. So they are exact for a threshold above -1000.0 and up to 1000.0, and the suggested threshold is always inside the window and can be applied.
36. A section "Favorite authors" shows the number of favorite authors and lists them, those with the most Panda favorites first, each with its favorite count and its effective weight. It shows 50 authors, "Show more" adds 50, and the page gets the 500 with the most favorites.
37. Each author of the list has the pin controls of a table row, so the user pins an author there.
38. The section holds the author bonus: a number field, "Save bonus", and "Turn off bonus" while it is on. It says whether the bonus is on. After a change the page calculates again, like after an override change.
39. Without an active model, the "Pinned weights" section says that pinned weights have no effect, and the "Favorite authors" section says that the author bonus has none.
40. A failed pin or bonus change shows a message beside the control that was used, leaves the pins and the bonus as they were, and starts no calculation.

Detail view:

41. When a local gallery with tags is shown, the view asks the collector for the weights of its tags. The request does not delay the page.
42. When the collector answers, each tag that the Panda catalog has shows its effective weight under the active model, or a dash when it has none, and a pinned tag has a pin mark.
43. The user pins a tag there: a number field that starts at the tag's current weight, "Pin", "Ignore" (a pin of 0) and, for a pinned tag, "Remove pin".
44. After a successful change the tags section shows the new weights.
45. A failed save shows a message and leaves the pin unchanged.
46. A tag that the Panda catalog does not have gets no pin control and shows the reason.
47. Without an active model, the tags section says that pinned weights have no effect. A tag then shows its pin, or the author bonus for a favorite author while the bonus is on, and a dash otherwise.
48. The tag selection and "Search selected tags" work as today.
49. When the collector is not configured, not reachable, fails, or gives an answer that is no tag weights answer, the view looks as it does today: no weights, no pin controls, no message.
50. Only tags get a pin in the detail view, not the uploader and not the category.

Local API:

51. `POST /api/collector/catalog/score/pins` with `{"kind","value","weight"}` returns 200 with the stored pin `{"id","kind","label","weight"}`. 400 `invalid_pin` for an unknown kind, an empty value, a tag without `namespace:value`, an unknown category, a missing weight or a weight beyond the bound of behaviour 5. 404 `tag_not_found` for a tag the Panda catalog does not have. `DELETE /api/collector/catalog/score/pins/{id}` returns 204, also when no such pin exists, and 400 `invalid_pin` for an ID that is not a positive integer.
52. `PUT /api/collector/catalog/score/author-bonus` with `{"bonus": <number>}` sets the bonus and returns 204. `{"bonus": null}` turns it off. 400 `invalid_author_bonus` for a number that is not above 0 or is beyond the bound of behaviour 5.
53. `POST /api/collector/catalog/score/tags` with `{"tags": ["namespace:value", ...]}` returns 200 with `{"active", "tags"}`: one entry per sent tag, in the sent order (see **Design**).
54. The preview carries the pins, the author bonus and the favorite authors (see **Design**). A catalog page carries a `source` on each summary tag and score weight whose weight is a pin or the bonus, and `favorite_author` on each summary tag of a favorite author.
55. All these routes answer 503 `collector_not_configured` without a collector, 502 `collector_unauthorized` when the collector rejects Tana's token, and 502 `collector_unavailable` when it can't be reached or fails.

## Design

**Data flow.** The browser talks only to the local service. Each new local route calls one `collectorapi.Client` method, which calls the collector route of the same name under `/api/catalog/score/...`, which calls one `catalog.Service` method. Non-test code in `internal/local/` reaches the collector only through `internal/collectorapi`. Pins, the author bonus and the set of favorite authors are collector state (ADR 0019). The listing reads them inside the catalog page query, the preview reads them when it calculates, and the detail view reads them through the tag weight lookup. The set of favorite authors is written by the `favorites` package when a sync completes and read by the `catalog` package.

Signatures, payloads, schema and the stated constraints are fixed. Query shapes, private helpers, file names and test sequences are the implementer's. A line that starts with "Hint:" is optional.

### The weight rule

For a model M (the active model, a candidate, or no model) the effective weight of a value is:

- **Tag:** its pin, when it has one. Else, when the author bonus B is on and the tag is a favorite author: the larger of its calculated weight under M and B, and B when M has no weight for it. Else its calculated weight under M. Else it has none.
- **Uploader, category:** its pin, when it has one. Else its calculated weight under M. Else none.
- **Source:** `pin` when the pin gives the weight. `author_bonus` when B gives it, which is when the tag has no pin, is a favorite author, B is on, and M has no weight for the tag or a weight below B. Empty otherwise.

"No model" means no calculated weights: only pins and the bonus give a weight. The rule is implemented in two places and nowhere else:

- **In SQL**, by the private builder of ticket 01 in `internal/collector/catalog`. It serves `scoreSum` (and through it `scoreTenths`, `belowThreshold`, `markScores` and `scoreFilter.recent`), `markWeights`, `summarizeTags` and the tag weight lookup. Each part probes by key: a pin by `panda_score_pins` (tag ID, or kind and value), a favorite author by the primary key of `favorite_authors`, a calculated weight as today. Nothing in it reads `favorites`.
- **In Go**, by one private function in the same package, for the scores that are calculated in Go: `heldOutTenths` and the effective weights of the preview's favorite author list.

Whether a tag is a favorite author is a fact of its own, apart from its weight and its source: a tag is one when `favorite_authors` holds it, with the bonus on or off.

Hint: while no pin exists and the bonus is off, the builder can return today's expression, so the listing runs the statements it runs today.

### Limits

Two private constants of `internal/collector/catalog`, each with a comment on its purpose:

```go
// The histograms of a preview count scores from -1000.0 to 1000.0. (ticket 02)
const scoreWindowTenths = 10000

// The largest magnitude of a pin and of the author bonus. The sum of a
// gallery's weights, times ten, then stays a finite float. (ticket 03)
const maxScoreWeight = 1e300
```

- **The window.** The tenths that a histogram counts are `floor(score * 10)` limited to `[-scoreWindowTenths, scoreWindowTenths]`. So a histogram has at most `2*scoreWindowTenths + 1` counts, its lowest bucket also counts every lower score, and its highest bucket every higher one. For a threshold `t` with `-scoreWindowTenths < t <= scoreWindowTenths`, the histograms and the listing agree as they do today. `suggestedThreshold` returns a bucket of a histogram or 0, so it is inside the window, and `validScoreThreshold` accepts it. The listing itself has no window: `belowThreshold` compares the unlimited `floor(score * 10)` with the threshold.
- **The bound.** `SetScorePin` and `SetAuthorBonus` refuse a number that is NaN or whose magnitude is above `maxScoreWeight`. Stored weights are then finite, and no sum of them overflows in PostgreSQL or in Go. No other code checks the bound: the browser sends every finite number.

### Collector schema (`internal/collector/storage/migrations`)

Each migration carries a comment on its purpose, in the style of `0005_panda_score_overrides.sql`. Index names are the implementer's. Migrations are numbered without a gap, so the three tickets build in this order.

- `0008_panda_score_pins.sql` (ticket 03): table `panda_score_pins` with `id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY`, `kind TEXT NOT NULL CHECK (kind IN ('tag', 'uploader', 'category'))`, `tag_id BIGINT REFERENCES catalog_tags (id)`, `value TEXT NOT NULL DEFAULT ''`, `weight DOUBLE PRECISION NOT NULL`, `created_at TIMESTAMPTZ NOT NULL`. Checks as in `panda_score_overrides`: a tag row has `tag_id` and an empty `value`, an uploader or category row has no `tag_id` and a non-empty `value`. Unique: `tag_id` among tag rows, `(kind, value)` among the others. These two indexes serve the pin probe of the score expression. No reference to a model and none to a gallery row.
- `0009_favorite_authors.sql` (ticket 06): table `favorite_authors` with `tag_id BIGINT PRIMARY KEY REFERENCES catalog_tags (id)` and `favorites INTEGER NOT NULL`. The migration fills it once from the stored favorites, by the definition of behaviours 12 and 13 (behaviour 17).
- `0010_panda_score_settings.sql` (ticket 07): table `panda_score_settings` with `id INTEGER PRIMARY KEY CHECK (id = 1)` and `author_bonus DOUBLE PRECISION CHECK (author_bonus > 0)`, and its one row with a NULL bonus, like `favorite_download_settings`. NULL means off.

### `internal/collectorapi` (`catalog_score.go`, `catalog.go`)

```go
// The sources of an effective weight that the model did not calculate.
const (
	ScoreSourcePin         = "pin"          // ticket 04
	ScoreSourceAuthorBonus = "author_bonus" // ticket 07
)

// ScorePin is one pinned weight, labelled like an override. (ticket 03)
type ScorePin struct {
	ID     int64   `json:"id"`
	Kind   string  `json:"kind"`
	Label  string  `json:"label"`
	Weight float64 `json:"weight"`
}

// ScoreFavoriteAuthor is one favorite author: its tag as namespace:value,
// the number of Panda favorites that carry it, and its effective weight
// under the candidate, nil without one. (ticket 08)
type ScoreFavoriteAuthor struct {
	Label     string   `json:"label"`
	Favorites int      `json:"favorites"`
	Weight    *float64 `json:"weight"`
}

// ScoreTagWeights answers a tag weight lookup. Active says whether a Panda
// score model is active. (ticket 09)
type ScoreTagWeights struct {
	Active bool             `json:"active"`
	Tags   []ScoreTagWeight `json:"tags"`
}

// ScoreTagWeight is the answer for one sent tag. Known says whether the Panda
// catalog has the tag. Weight is its effective weight, nil without one.
type ScoreTagWeight struct {
	Tag    string    `json:"tag"`
	Known  bool      `json:"known"`
	Weight *float64  `json:"weight"`
	Pin    *ScorePin `json:"pin,omitempty"`
}
```

New fields on existing types:

- `CatalogTag.Source string` and `CatalogScoreWeight.Source string`, both `json:"source,omitempty"` (ticket 04): `ScoreSourcePin`, `ScoreSourceAuthorBonus` or empty. `CatalogTag.Weight`, `CatalogScoreWeight.Weight` and `CatalogItemScore.Value` hold effective weights and their sum.
- `CatalogTag.FavoriteAuthor bool` with `json:"favorite_author,omitempty"` (ticket 07): the tag is a favorite author. It is set only while a model is active, like `Weight`.
- `ScorePreview.Pins []ScorePin` with `json:"pins"` (ticket 05): every pin, by ID, never nil.
- `ScorePreview.AuthorBonus *float64` with `json:"author_bonus"`, `ScorePreview.FavoriteAuthorCount int` with `json:"favorite_author_count"`, and `ScorePreview.FavoriteAuthors []ScoreFavoriteAuthor` with `json:"favorite_authors"`, never nil (ticket 08).

`ScoreHistogram` keeps its shape. Its comment gains the window: the first and the last bucket of a histogram that reaches the window's edge also count the scores beyond it (ticket 02).

Client methods:

```go
func (c *Client) SetScorePin(ctx context.Context, kind, value string, weight float64) (ScorePin, error) // POST /api/catalog/score/pins, expects 200 (ticket 03)
func (c *Client) RemoveScorePin(ctx context.Context, id int64) error                                     // DELETE /api/catalog/score/pins/{id}, expects 204 (ticket 03)
func (c *Client) SetAuthorBonus(ctx context.Context, bonus *float64) error                               // PUT /api/catalog/score/author-bonus, expects 204 (ticket 07)
func (c *Client) ScoreTagWeights(ctx context.Context, tags []string) (ScoreTagWeights, error)             // POST /api/catalog/score/tags, expects 200 (ticket 09)
```

`ScoreTagWeights` returns an error for an answer whose number of entries differs from the number of sent tags. Other failures are `*HTTPError` values, as for the other score methods.

### Collector: `internal/collector/catalog`

**Ticket 01.** One private builder in `score_filter.go` returns the SQL expression for the calculated weight of a tag (named by an expression for its `catalog_tags` ID), of an uploader and of a category under the model whose ID a placeholder binds: NULL without a weight. `scoreSum`, `markWeights` and `summarizeTags` take every weight from it, and none of them names `panda_score_weights` itself. `markWeights` keeps its result: the values that have a weight, the strongest `scoreItemWeights`. The scalar probe per tag stays (see the comments on `scoreSum` and in `summarizeTags`).

**Ticket 02.** `scoreWindowTenths` and the window of **Limits**. `scoreFilter.recent` limits the tenths of the candidate's and the active model's score in SQL, before the cast to `integer`. `heldOutTenths` limits them in Go, before the conversion to `int`. `scoreHistogram`, `suggestedThreshold`, `scoreFilter.heldOut`, `belowThreshold`, `hidden` and `markScores` do not change.

**Pins (ticket 03).**

- `var ErrInvalidScorePin = errors.New("invalid score pin")`.
- `func (s *Service) SetScorePin(ctx context.Context, kind, value string, weight float64) (collectorapi.ScorePin, error)`. The kind and the value have the rules of `SetScoreOverride`, from one private helper that both use: the value is trimmed, a tag is cut at its first colon and matched on `catalog_tags (namespace, value_lower)` with the folding of `replaceTags`, a category goes through `panda.NormalizeCategories`, an uploader is kept as given. Where `SetScoreOverride` returns `ErrInvalidScoreOverride`, `SetScorePin` returns `ErrInvalidScorePin`. A tag that is not in `catalog_tags` returns `ErrTagNotFound`. A weight that is NaN or whose magnitude is above `maxScoreWeight` returns `ErrInvalidScorePin`. An existing pin for the same tag, uploader or category keeps its row and ID and gets the new weight. The result is the stored pin with its label: `namespace:value` in the tag's stored spelling, or the stored value. The method takes no lock on `panda_score_models`.
- `func (s *Service) RemoveScorePin(ctx context.Context, id int64) error`. `id <= 0` returns `ErrInvalidScorePin`. A missing row is not an error.
- A private reader of all pins with their labels and tag IDs, ordered by ID, for the score filter and the preview.

**Pins in the listing (ticket 04).** The builder of ticket 01 gives the effective weight under pins, and its source. `List` needs no other change: `hidden`, `belowThreshold`, `markScores`, `markWeights` and `summarizeTags` follow the builder.

- `belowThreshold` stays `false` without an active model, and `inForce` does not look at pins (behaviour 9).
- `markWeights` lists every value of the gallery that has an effective weight, also a pin of 0 and a pinned value without a calculated weight, each with its `Source`, and keeps the strongest `scoreItemWeights` by absolute weight.
- `summarizeTags` ranks by the effective weight (`tagRank` reads `Weight`, so a pin of 0 ranks before a tag without a weight) and sets `Source`. Override tags keep the first group and also carry `Weight` and `Source`.
- Without an active model, `summarizeTags` and `markScores` attach no weight and no source, as today.

**Pins in the preview (ticket 05).** `PreviewScoreModel` fills `Pins`. `heldOutTenths` scores each evidence gallery with the effective weights of its fold (the Go side of the weight rule) and keeps the window of ticket 02. `scoreFilter.heldOut`, `suggestedThreshold` and `scoreFilter.recent` need no change of their own: the first two read the held-out scores, and `recent` scores through `scoreTenths`. `Candidate.Weights` keeps the calculated weights and their order.

**The author bonus (ticket 07).**

- `var ErrInvalidAuthorBonus = errors.New("invalid author bonus")`.
- `func (s *Service) SetAuthorBonus(ctx context.Context, bonus *float64) error`. `nil` turns the bonus off. A number that is not above 0, is NaN, or is above `maxScoreWeight` returns `ErrInvalidAuthorBonus`.
- The score filter reads the bonus with the pins. The builder adds the favorite author part of the weight rule and the source `author_bonus`. It probes `favorite_authors` by `tag_id` and never reads `favorites`.
- While a model is active, `summarizeTags` sets `FavoriteAuthor` on each tag that `favorite_authors` holds, with a probe by `tag_id`. The flag does not depend on the bonus, on a pin or on an override of the tag. Without an active model no tag has it.

**Favorite authors in the preview (ticket 08).** `PreviewScoreModel` fills `AuthorBonus`, `FavoriteAuthorCount` (the number of rows of `favorite_authors`) and `FavoriteAuthors`: the `scorePreviewAuthors = 500` authors with the most favorites, more favorites first, then by label. Each `Weight` is the effective weight under the candidate, by the Go side of the weight rule with the candidate's weights. Without a candidate it is the pin or the bonus alone, and nil when the tag has neither. The Go side of the rule gains the bonus, so the held-out scores count it.

**Tag weight lookup (ticket 09).** `func (s *Service) ScoreTagWeights(ctx context.Context, tags []string) (collectorapi.ScoreTagWeights, error)`.

- The answer has one entry per element of `tags`, in the same order, with `Tag` as sent. An empty or nil list gives an empty, non-nil `Tags`.
- `Known` is true when the element is `namespace:value` and `catalog_tags` has the tag, matched like a pin's tag. An element without a colon or with an empty part is not known.
- For a known tag, `Weight` is the effective weight under the active model, and under no model when none is active. `Pin` is the tag's pin, if it has one.
- `Active` says whether a model is active.
- Hint: resolve all tags in one statement and read the weights with the builder, with a NULL model ID when no model is active.

### Collector: `internal/collector/favorites` (ticket 06)

The package owns the table `favorite_authors` and its refresh, with queries in `queries.sql` (regenerate `dbgen`).

- `store.savePage` refreshes the set when a sync is done, in its transaction, after `ReconcileFavorites` and before any return of the completed branch: one statement adds the favorite authors of behaviours 12 and 13 and sets their favorite counts, and for a full sync (`current.Full`) one more removes every row that is no longer one. Nothing else writes the table.
- The definition: tags of the namespace `artist` that at least 2 distinct `gallery_id` values of `favorites` carry in `catalog_gallery_tags`. It counts the favorites of every stored collection.
- No lock is needed: `savePage` transactions run one after another, because each locks the `favorite_download_settings` row first.
- The `catalog` package reads `favorite_authors` with its own SQL, as it reads `favorites` today, and imports nothing from `favorites`.

### HTTP

| Collector route (`internal/collector/httpapi`) | Local route (`internal/local/httpapi`) | Ticket |
|---|---|---|
| `POST /api/catalog/score/pins` | `POST /api/collector/catalog/score/pins` | 03 |
| `DELETE /api/catalog/score/pins/{id}` | `DELETE /api/collector/catalog/score/pins/{id}` | 03 |
| `PUT /api/catalog/score/author-bonus` | `PUT /api/collector/catalog/score/author-bonus` | 07 |
| `POST /api/catalog/score/tags` | `POST /api/collector/catalog/score/tags` | 09 |

- Requests and answers are those of behaviours 51 to 53, the same on both sides. The handlers decode with `server.DecodeJSON`. The pin body is `{"kind": string, "value": string, "weight": number}`, and a missing or null `weight` is an invalid pin. The bonus body is `{"bonus": number or null}`, and an absent `bonus` counts as null. The tag body is `{"tags": [string]}`. `server.DecodeJSON` reads at most 16 KiB, so a gallery whose tag list is larger gets no answer, and its detail view looks as today.
- `catalogError` maps `ErrInvalidScorePin` to 400 `invalid_pin` and `ErrInvalidAuthorBonus` to 400 `invalid_author_bonus`. `ErrTagNotFound` is 404 `tag_not_found` already.
- The local handlers follow `HandleCollectorSetScoreOverride` and `HandleCollectorRemoveScoreOverride`: `catalogCollectorConfigured` first, a pin ID that is not a positive integer is refused with 400 `invalid_pin` without a collector request, and `writeCatalogCollectorError` passes 400 `invalid_pin` and 400 `invalid_author_bonus` through.
- The catalog page and the preview need no handler code: the new fields are on the shared structs.

Test seams: package tests in `internal/collector/catalog` with `openCatalog`, `retain`, `setVerdict`, `setOverride`, `scorePage`, `itemByID`, `seedVerdicts`, `seedScoreEvidence`, `seedScoredGalleries`, `applyCandidate`, `previewScore`, `weightOf`, `histogramCounts`, `assertSummaryTags`. Package tests in `internal/collector/favorites` with `testStore`, `seed`, `admit`, `syncEntries`, `manualService`, `drain`. Validation tables in `internal/collector/httpapi/catalog_test.go`, like `TestScoreOverrideValidation`. Local tests in `internal/local/httpapi/catalog_test.go` through the real collector handler behind `httptest.NewServer`, like `TestLocalScoreOverrides`. The migration that fills `favorite_authors` is tested in `internal/collector/storage/storage_test.go`, like `TestDisplayEvidenceMarksDisplaysFromBeforeVerdicts`. Each core behaviour is pinned once, at the package seam. A local test pins what the local route adds: the route, the passed error codes, and one JSON key per new field.

### Web

`web/src/panda/score.ts`:

```ts
export type ScoreSource = 'pin' | 'author_bonus'                                   // ticket 10
export interface ScorePin { id: number, kind: ScoreKind, label: string, weight: number } // ticket 11
export interface ScoreFavoriteAuthor { label: string, favorites: number, weight: number | null } // ticket 13
export interface ScoreTagWeight { tag: string, known: boolean, weight: number | null, pin?: ScorePin } // ticket 14
export interface ScoreTagWeights { active: boolean, tags: ScoreTagWeight[] }       // ticket 14

export const savePandaScorePin = (pin: { kind: ScoreKind, value: string, weight: number }, signal: AbortSignal) => Promise<ScorePin> // ticket 11
export const removePandaScorePin = (id: number, signal: AbortSignal) => Promise<void>   // ticket 11
export const savePandaAuthorBonus = (bonus: number | null, signal: AbortSignal) => Promise<void> // ticket 13
export const getPandaTagWeights = (tags: string[], signal: AbortSignal) => Promise<ScoreTagWeights> // ticket 14
```

`ScorePreview` gains `pins: ScorePin[]` (ticket 11) and `author_bonus: number | null`, `favorite_author_count: number`, `favorite_authors: ScoreFavoriteAuthor[]` (ticket 13). The pin functions use `collectorMessages` plus `tag_not_found` → "No gallery in the Panda catalog has this tag." and `invalid_pin` → "Enter a tag as namespace:value, an uploader name, or a Panda gallery category.", the texts of the override form. The bonus function adds `invalid_author_bonus` → "Enter a number above 0." `ScoreHistogram`, `currentHiddenShare`, `hiddenShare` and `recentHiddenShare` do not change: the histograms they read already count the pins and the bonus, inside the window. A number field "holds a number" when its text is a finite number.

**Cards (ticket 10, `web/src/panda/api.ts`, `PandaCard.tsx`).** `PandaSummaryTag.source?: ScoreSource`, `PandaSummaryTag.favorite_author?: boolean` and `PandaScoreWeight.source?: ScoreSource`.

- `stripTag` gives a tag at most one mark, the first that applies:
  1. a tag with `override`: as today, the override's mark and colour;
  2. a tag with `source: 'pin'`: `DrawingPinFilledIcon` followed by visually hidden "Pinned: ";
  3. a tag with `favorite_author`: `StarFilledIcon` followed by visually hidden "Favorite author: ", whatever its `source` is (`@radix-ui/react-icons`).

  A tag of case 2 or 3 keeps the colour that its weight gives it by `clearTagWeight`.
- `PandaDetails`: each weight line ends with " (pinned)" for `source: 'pin'` and " (author bonus)" for `source: 'author_bonus'`.
- `GalleryBox.tsx` needs no change: its key already covers whether a tag has a mark. `PandaCatalog.tsx` needs no change.

**Evaluation page (`web/src/panda/PandaScorePage.tsx`).** The page keeps its one table: the pin list and the favorite author list are lists. The texts in quotation marks are fixed.

- *Pin state (ticket 11).* The page holds the pins in its own state. Each preview replaces them with `preview.pins`. A saved pin's answer replaces the pin of the same kind and label or is added, and a removed pin leaves the list.
- *Pin changes (ticket 11).* They run through the page's one `useMutation`, so no two changes run at once, and the pin controls are disabled only while a change is being saved, not while a calculation runs. After a successful pin change the page calculates again with the favorites value of its latest preview request, which aborts a calculation in progress. A pin change never alters whether the next preview starts the threshold fields again. A failed pin change shows "Could not change the pin." with the error and requests no preview.
- *Pin controls (ticket 11).* One component, used by the weights table and the favorite author list: a number field with the accessible name "Pinned weight of <label>" (the label as `weightLabel` gives it) and `step="any"`, a button "Pin" that is disabled while the field holds no number, a button "Ignore", and a button "Remove pin" only for a pinned value. Enter in the field does what "Pin" does. The field shows the pin's number (`String(weight)`) whenever the pin changes, and is empty without a pin.
- *Weights table (ticket 11).* A column "Pin" after the column "Override" holds the pin controls of the row's kind and label. The first six columns stay as they are. A failed change of a row shows its message in the "New model" section, like a failed override change of a row.
- *Section "Pinned weights" (ticket 11),* after "Overrides", shown also without a candidate and without an active model: without an active model the text "No score model is active: pinned weights have no effect.", then the list of all pins, each as `weightLabel` and `formatWeight` of its number ("female:glasses +2.00", "Uploader Some uploader -1.50") with a button "Remove", or "No pinned weights." Messages of the list and of the form show in this section.
- *Form "Add pin" (ticket 12),* in that section, with the accessible name "Add pin": the kind (Tag, Uploader, Category), the value field or the category choice with the labels of the override form, a number field "Weight", and a button "Add pin" that is disabled while the value is empty or the weight is no number. A successful add clears the value and the weight.
- *Section "Favorite authors" (ticket 13),* after "Pinned weights", shown also without a candidate:
  - the text "<count> favorite authors: artist tags that at least 2 Panda favorites carry.";
  - without an active model, "No score model is active: the author bonus has no effect.";
  - a number field "Author bonus" (step 0.1) that starts at the saved bonus and is empty while it is off, a button "Save bonus" that is disabled unless the field holds a number above 0, and a button "Turn off bonus" while the bonus is on. Beside them "The author bonus is off." or "Each favorite author's tag weighs at least <formatWeight(bonus)>.";
  - the list of `favorite_authors`: the label, "<n> favorites", the weight (`formatWeight`, or "–" for null) and the pin controls for the kind `tag` and the label. It shows 50, and "Show more" adds 50. A failed pin change from the list shows its message in this section.
  - A bonus change runs like an override change: through `runChange`, disabled during a calculation, then a new calculation that keeps the threshold fields. A failure shows "Could not save the author bonus." with the error.

**Detail view (ticket 14, `web/src/galleries/GalleryDetail.tsx`).**

- After the gallery is loaded, and only when it has tags, the view calls `getPandaTagWeights` with each tag as `namespace:value`.
- The view takes an answer only when it is a valid tag weights answer: `typeof answer.active === 'boolean'` and `Array.isArray(answer.tags)`. A failed request and every other answer leave the view as it is today: no weight, no button, no notice and no message. The view uses an entry only for the tag whose `namespace:value` equals the entry's `tag`, and a tag without such an entry looks as today.
- With a valid answer and `active === false`, the tags section shows "No score model is active: pinned weights have no effect."
- A tag with `known: true` gets a button beside its toggle, in the same list item and outside the toggle's link. Its text is the weight with sign and two decimals, or "–" for null. Its accessible name is "Weight of <namespace:value>: <text>", and for a pinned tag "Pinned weight of <namespace:value>: <text>", with `DrawingPinFilledIcon` before the text.
- The button opens a Radix `Popover` with the pin editor: a number field "Weight" and the buttons "Pin" (disabled while the field holds no number), "Ignore" and, for a pinned tag, "Remove pin". The field starts at the pin's number as stored for a pinned tag, at the weight rounded to two decimals for another tag with a weight, and at 0 for a tag without one. The buttons call `savePandaScorePin` with the kind `tag` and the tag's `namespace:value`, and `removePandaScorePin`.
- After a successful change the editor closes and the view requests the tag weights again. A failed change shows "Could not change the pin." with the error in the editor, which stays open.
- A tag with `known: false` gets no button. Its list item carries the visually hidden text " (not in the Panda catalog)" and an icon with the title "Not in the Panda catalog".
- The toggle link, `toggleTag`, "Search selected tags" and "Clear" do not change. `GalleryPandaLink` does not change.

Test seams: `web/src/panda/PandaCard.test.tsx` renders `<Theme><PandaCard /></Theme>`. `web/src/panda/PandaScorePage.test.tsx` renders `<App />` at `#/panda/score` with a stubbed `fetch`, and its fixtures `preview()` and `measured()` gain the new fields with empty defaults. `web/src/galleries/GalleryDetail.test.tsx` renders `<GalleryDetail />` with a stubbed `fetch`.

## Assumptions

- **The histograms count inside a window of scores from -1000 to 1000** (`scoreWindowTenths`), and stay dense. For a threshold outside the window the evaluation page's shares treat a score beyond the window as the window's edge. Rejected: sparse histograms, which are exact for every threshold but are a large prefactor (**Fit**, item 2), and a wider window, which makes the preview's answer larger for no threshold in use. The listing has no window.
- **A pin and the author bonus have a technical bound of 1e300 in magnitude.** The brief permits each number and excludes a limit for negative weights. This bound is where a sum of floats stops working in PostgreSQL, far beyond any number a user sets. Rejected: no bound, where one pin near the largest float fails every catalog page that meets its value and the preview, so the user could not remove it on the evaluation page.
- **A number beyond the bound is refused with the general message of its control** (`invalid_pin`, `invalid_author_bonus`). Rejected: an error code and a text of its own for a number nobody enters.
- **The star marks every favorite author's tag on a card while a model is active**, with the bonus on or off, unless the tag has a pin or an override. The hover details name "(author bonus)" only where the bonus gives the weight.
- **The star needs an active model**, like the weights and the pin mark. Rejected: a star without a model, which would be the one score mark on a strip that is not ranked, and would make the listing a page that needs the notice about pins.
- **The bonus is a number above 0.** "At least the bonus" has no clear meaning for a negative bonus on a tag without a calculated weight, and 0 beside "off" would be a third state. Rejected: any number.
- **The set of favorite authors is a stored table that only a completed sync refreshes.** Rejected: reading it from `favorites` for each catalog page (an aggregate over all favorites before every page), a test per candidate (the brief's trap), and a refresh with every projected gallery (it would remove authors outside a sync, against the guard).
- **The `favorites` package owns `favorite_authors`.** The set is a fact about the favorites, and that package completes the sync. Rejected: a function of the `catalog` package that `savePage` calls, which adds an import for one statement.
- **A sync that is not a full re-sync never removes an author**, also when a favorite's tags changed in the catalog. The author then stays with its last count until the next completed full re-sync. Rejected: replacing the set on every completed sync, which lets an incremental sync remove an author.
- **An initial sync of a collection counts as a full re-sync**, as it does in `savePage` today (`current.Full`).
- **The migration fills the set.** Rejected: an empty set until the first sync after the update.
- **A favorite counts with provisional Panda metadata** when the feed gave an `artist` tag: the catalog has its tags then.
- **Favorites of every stored collection count**, also those of another Panda account, as counted favorites do in the preview.
- **Cards show pins only with an active model**, so the listing shows no pin without one and needs no notice. The evaluation page and the detail view carry the notice of the brief.
- **Pins do not put the score filter in force.** Rejected: a filter control in the listing for pins without a model, where they hide nothing.
- **The pin route answers 200 with the stored pin**, unlike the override route (204). The evaluation page needs the ID and the label without waiting for a calculation.
- **A pin change replaces a running calculation by aborting its request.** No timer and no delay constant. Rejected: waiting for each calculation (the brief's trap) and a debounce (a timing constant, and the last pin would still wait).
- **A bonus change waits for the calculation like an override change.** The bonus is one setting, not a series of changes.
- **The weights table has a "Pin" button** beside the brief's number field, "Ignore" and "Remove pin": a number field needs an action that saves it. "Remove pin" shows only on a pinned row.
- **The favorite author list shows the effective weight under the candidate**, like the weights table shows the candidate's weights. Rejected: the active model's weight, and both.
- **The page gets the 500 favorite authors with the most favorites** and the count of all, like the 500 strongest weights. A further author is pinned with the form.
- **The detail view opens the pin editor in a popover** from a button beside each tag. Rejected: controls inside each tag, which would break the toggle, and one shared editor below the list, which needs a second selection.
- **The detail view marks a tag that the Panda catalog does not have before the user tries**, from the lookup's `known`. A save can still fail with `tag_not_found`, and shows the same reason.
- **The detail view asks for the weights again after a change.** Rejected: patching from the pin's answer, which does not give the weight after a removal.
- **Without an active model the detail view shows pins and the bonus as the weights** they would have, with the notice.
- **The browser sends the tag list to the lookup route.** Rejected: a local route by gallery ID that reads the tags itself, which gives a collector route a dependency on the gallery store.
- **`invalid_pin` is an error code of its own** with the override form's text. Rejected: `invalid_override` on a pin route.
- **No version check between Tana and the collector.**
- **Terms.** `CONTEXT.md` gains **Pinned weight** and **Favorite author**, which also defines the author bonus, and **Panda gallery score** names both. "Effective weight", "calculated weight", "source", "strip", "candidate" and "window" are words of this spec. ADR 0019 gains a paragraph instead of a new ADR.

## Out of scope

- A pin action on Panda cards.
- A "must see" rule for favorite authors, and any change to the order of the hiding rules.
- `group` tags, or any namespace but `artist`, as favorite authors.
- A bonus that grows with the favorite count, a bonus per author, and a setting for the minimum of 2.
- A limit for pins or for the bonus below the technical bound, and any change to the weight calculation.
- Exact numbers on the evaluation page for a threshold outside the window, and a note on the page about the window.
- Pins for the uploader or the category in the detail view, and weights or pins in the local gallery list.
- An update of loaded cards or of an open detail view when a pin changes elsewhere.
- A refresh of the favorite authors outside a completed sync, and a favorite author mark in the detail view.
- A history of pins, and pins per model.

## Manual checks

- **Score window.** On the evaluation page against the real collector, pin a common tag to -100000 and another to 100000. Good: the page loads about as fast as before, its shares move as the pins say, and the suggested threshold stays between -1000.0 and 1000.0. Then remove both pins and compare the scores in the hover details of some cards with the window: real scores stay far inside it. The constant to tune is `scoreWindowTenths`: wider when real scores come near ±1000, narrower when the preview's answer gets slow to load.
- **Listing speed with pins and the bonus.** With the real catalog, an active model, some pins and the bonus on, open the listing in "Filter on", "Hidden only" and the unseen view. Good: pages load about as fast as without pins. There is no constant to tune: a slow page means that the score expression does not probe by key (**Design → The weight rule**).
- **Several pins in a row.** On the evaluation page against the real collector, pin three rows one after the other without waiting. Good: each row and the list show the pin at once, the spinner runs once through, and the numbers appear about one calculation after the last pin. No constant: a wait per pin means that the aborted request does not stop the collector's calculation.
- **Width of the weights table.** Look at the table with the pin column on a wide and a narrow window. Good: the pin field and its buttons stay on the row, and the table scrolls sideways before a cell breaks into three lines.
- **Marks on cards.** Look at cards with a pinned tag, a favorite author's tag and an override tag, also with long values. Good: the pin, the star and the two eyes are easy to tell apart at card size, the badge height is the same, and the strip keeps its height.
- **Detail view.** Open a local gallery with many tags, with the collector reachable. Good: the weight buttons sit beside their tags without breaking the rows apart, the popover opens next to its tag, a tag that the Panda catalog does not have is easy to tell from one without a weight, and a click on a tag still selects it.
- **Sync with the real favorites.** Run "Sync all". Good: each sync completes without a visible pause at its end, and the count on the evaluation page follows.
- **The update.** Start the collector on a copy of the real database. Good: migration 0009 runs in seconds, and the evaluation page shows a plausible number of favorite authors before any sync.

The plan picks no timing constant: no delay, no debounce and no polling interval.
