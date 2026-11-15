# Quick pin edits, single-row score lists, and tag completion on the score page

## Problem

On the score page (`#/panda/score`) the user works through three long lists: up to 500 weights of the new model, the pinned weights, and up to 500 favorite authors. Four things slow this down today.

- A row breaks over several lines. The pin controls are a number field with three text buttons, the override buttons take more width, and both wrap. A list of 50 rows fills several screens and is hard to scan.
- A pinned weight can be changed in one place only: its row in the weights table. That row exists only when the value is among the 500 strongest weights of the new model and the user has paged to it. The "Pinned weights" list offers "Remove" and nothing else, so changing a pin on any other value means typing its value and the new number into the "Add pin" form again.
- Nothing finds a tag in the weights table or an artist among the favorite authors, except "Show more" and reading.
- "Add pin" and "Add override" take the tag as free text that has to be exactly right. A typo answers "No gallery in the Panda catalog has this tag." The search field of the Panda listing completes tags from the Panda catalog and knows the namespace short forms (`f:` for `female:`). These two forms do neither.

## Solution

Every entry of the three lists stays on one line. The label takes the width that is left and ends with an ellipsis when it is too long, and hovering shows it in full. The pin controls of the weights table and of the favorite authors list become a number field with three small icon buttons, and Enter in the field pins.

The "Pinned weights" list becomes the place to change a pin: each row has a number field with the pinned weight. Enter saves it, and a save button shows while the field differs from the stored weight. The row also shows the weight that the new model calculates for the value, so the user sees what the pin replaces. A recalculation leaves text alone that the user is typing in a pin field.

The weights table and the favorite authors list each get a text filter over all entries that the page holds, not only the 50 shown.

The tag field of "Add pin" and of "Add override" completes tags from the Panda catalog exactly as the search field of the Panda listing does, with the same keys. A chosen suggestion puts the plain tag in the field. A pin or an override typed with a namespace short form is saved for the full tag.

## Fit

Fits after one small prefactor (ticket 01). Parts of the design that exist only because of how the code is built today:

1. **`GallerySearch.tsx` is the completion field and the search form in one component.** It navigates on submit, replaces the completed term with search syntax, and uses fixed element ids (`gallery-search`, `gallery-tags`, `gallery-tag-<n>`), so it can be on a page once. Its popup styles live in `Galleries.css`, which the score page does not load. *Prefactor (ticket 01):* the field with its suggestions moves into `TagCompletionField`, which takes its value, its completion request and its element id from the caller and leaves submission to the form around it. `GallerySearch` keeps the form, the navigation and the "Search" button. No behaviour changes, and no existing test is edited.
2. **A preview carries the 500 strongest weights and the 500 favorite authors with the most favorites** (`scorePreviewWeights`, `scorePreviewAuthors` in `score_model.go`), not all of them. Two consequences:
   - *Kept:* the filters search what the preview carries, as the brief says ("all weights in the preview"). When the preview carries fewer entries than exist, the count line of a filtered list says so (behaviour 25). Rejected: a filter that the collector applies to all weights of the model. It needs a request per filter change against a calculation that takes seconds, or a second read route beside the preview.
   - *Addition (ticket 03):* the page cannot look up the model weight of a pinned value in those 500 weights, because a pin is typically set on a value with a weak weight or none. The preview therefore gives each pin the weight that the candidate calculated for its value (`model_weight`), read from all weights of the candidate.
3. **The pin route answers with the stored pin, and the page puts that answer into its pin list before the calculation answers** (spec `panda-score-pins`, Fit 8). The answer has no model weight. *Fits (ticket 05):* the model weight belongs to the preview, not to the pin. The weights of the new model do not depend on the pins, so the weight that the last preview gave for a value stays right while a pin of that value is saved, replaced or removed. Each row of the "Pinned weights" list reads its model weight from the pins of the last preview, by kind and label. The pin state and `savePin` stay as they are: a saved pin keeps its model weight, and a pin that is new in this visit shows it when the calculation that follows answers.
4. **`parseScoreTarget` (`score_overrides.go`) serves three callers:** `SetScorePin`, `SetScoreOverride` and `ScoreTagWeights`, the lookup of the gallery detail view. The first two take what the user types. The third takes the tags of a local gallery, whose namespace is a real namespace and never a short form. *Kept inside ticket 02:* only the two save paths expand a short form. The lookup keeps matching by the namespace as sent, so the detail view stays as it is.
5. **The weights table keeps its number of shown rows in `PandaScorePage` (`shown`, passed through `Preview`), and the favorite authors list keeps its own.** A filter text belongs beside the number of shown rows, because changing one resets the other. *Kept inside ticket 06:* both lists keep the filter text and the number of shown rows in their own component (`Weights`, `FavoriteAuthors`), and `shown`/`onShown` leave `PandaScorePage` and `PreviewProps`. Both components stay mounted from one calculation to the next, so both values survive a recalculation; the existing test "the shown weights stay shown after an override calculates again" pins this and stays unedited.
6. **`PinControls` holds the rule that a pin field follows its pin and otherwise keeps the user's text** (the key `${pin.id}:${pin.weight}`). *Fits:* the editor of the "Pinned weights" list uses the same rule (ticket 05).
7. **Existing tests find the pin buttons by their accessible names "Pin", "Ignore" and "Remove pin", and a row's pin field by "Pinned weight of <label>".** *Kept:* the icon buttons keep these names, so the existing assertions stay. The field of the "Pinned weights" list gets a name of its own ("Weight of pin <label>"), because the same value can have a row in the table and a row in that list.
8. **`PandaScorePage.test.tsx` answers `fetch` calls from a queue in call order** (`respond`), and `changeCalls()` counts every call that is not a preview. A completion request from a tag field would take the next queued answer. *Changed in ticket 07:* the test file answers `/api/collector/catalog/completions` apart from the queue and leaves those calls out of `changeCalls()`.
9. **`Table.Root` of Radix Themes already wraps its table in a horizontal scroll area, and `.app-main` scrolls in both directions when its content is wider than it.** *Fits:* the table scrolls inside its frame once its cells stop wrapping. The two `<ul>` lists get a frame of their own that scrolls sideways, so that no row widens the page.
10. **Tana and the collector are deployed independently.** *Kept:* the user updates both together. Against an older collector the "Pinned weights" list shows no model weights, and a tag typed with a short form answers "No gallery in the Panda catalog has this tag."

## Behaviour

Terms: the *score page* is `#/panda/score` (earlier specs call it the evaluation page). The *listing* is the Panda catalog page (`#/panda`). A *pin* is a pinned weight, and an *override* is a score override. The *new model* is the Panda score model that the score page calculates and shows under "New model" before it is applied (the candidate). The *model weight* of a tag, uploader or category is the weight that the new model calculated for it, the number of the "Weight" column. A *recalculation* is the calculation that the page starts after a change. The *label* of a row is what the row shows for its value: a tag as `namespace:value`, an uploader as "Uploader <name>", a category as "Category <name>". A *plain tag* is `namespace:value`, without quotes and without `$`. A *short form* is the short name of a namespace in the search language: `a` artist, `c` character, `cos` cosplayer, `f` female, `g` group, `l` language, `loc` location, `m` male, `x` mixed, `o` other, `p` parody, `r` reclass.

Single-row lists:

1. The rule covers three lists of the score page: the weights table under "New model", the list under "Pinned weights", and the list under "Favorite authors". Every entry of the three lists stays on one line, at every window width.
2. The label takes the width that the other cells of its row leave free. A label that does not fit ends with an ellipsis.
3. The label carries its full text as its `title`, also when it fits, so hovering it shows the full value in the browser's tooltip.
4. No other cell wraps: the numbers, the override buttons, the pin controls, the favorite count, the model weight and "Remove" each stay on one line.
5. When the window is too narrow for a row whose label is 12rem wide, the list scrolls sideways inside its own frame. The page itself never scrolls sideways.

Pin controls in the weights table and in the favorite authors list:

6. Each row has, on its line, a number field and icon buttons. The field shows the pin of the row's value and is empty without one. The buttons are "Pin", "Ignore" and, only for a pinned value, "Remove pin". This holds for tags, uploaders and Panda gallery categories.
7. Each button shows an icon and no text. It has an accessible name ("Pin", "Ignore", "Remove pin") and a hover title ("Pin the entered weight", "Ignore: pin the weight 0", "Remove pin").
8. Enter in the field pins the field's number, as "Pin" does. Without a number in the field, Enter and "Pin" do nothing.
9. "Ignore" pins the weight 0, and "Remove pin" removes the pin. As today, a change shows at once in the row and in the "Pinned weights" list, and the page then calculates again.
10. The pin popover in the detail view of a local gallery stays as it is.

"Pinned weights" list:

11. Each row shows the label, a number field with the pinned weight, the model weight of the value as "Model <weight>" (for example "Model +0.50") when the new model has one, and "Remove". This holds for tag, uploader and category pins.
12. The model weight shows for every pinned value that the new model weighs, also for one that is not among the 500 weights of the table. Without a new model, or when it has no weight for the value, the row shows no model weight. A pin that is new in this visit shows its model weight when the calculation that follows answers.
13. Enter in the field saves the field's number as the weight of the pin.
14. A save button shows only while the field's text differs from the stored weight, and it saves like Enter. A text that is the same number as the stored weight ("2.0" for 2) does not differ: no button shows, and Enter sends nothing. A text that is no number differs: the button shows but is disabled, and Enter sends nothing.
15. Leaving the field does not save. The text and the save button stay.
16. After a save the field shows the stored weight, the save button goes, the row keeps its model weight, and the page calculates again, as after every pin change. A failed save shows "Could not change the pin." with the error in the section, keeps the text, and requests no calculation.
17. A recalculation never replaces the text of a pin field, whether in this list, in the weights table or in the favorite authors list. Only a change of that field's own pin does: when the pin gets another weight, is added or is removed, the field shows the pin again.
18. The list has no filter and no "Ignore" button: the user types 0 and saves. The list keeps its order: the pins as created, a new pin last.

Filtering:

19. The weights table and the favorite authors list each have one text field that filters their list. An entry matches when its label contains the text, ignoring case and spaces around the text. An empty text matches every entry.
20. The filter searches all entries that the preview carries (up to the 500 strongest weights, up to the 500 favorite authors with the most favorites), before the paging: the list shows the first 50 matches, and "Show more" adds the next 50 matches.
21. Changing the filter text starts its list again at the first 50 matches.
22. The filter text and the number of shown entries stay when a change of a pin, an override or the author bonus calculates again.
23. The count line says how many entries match. Weights table, with a filter text: "<shown> of <matches> matching weights, strongest first." Without one, as today: "<shown> of <all> weights, strongest first." Favorite authors list, with a filter text: "<shown> of <matches> matching favorite authors, most favorites first." Without one: "<shown> of <all> favorite authors, most favorites first."
24. When nothing matches, the list is replaced by "No weights match." (weights table) or "No favorite authors match." (favorite authors list).
25. While a filter text is set and the preview carries fewer entries than exist, the count line and the empty message end with the scope: "Only the <n> strongest of <all> weights are searched." or "Only the <n> authors with the most favorites of <all> are searched."
26. The text field is the only filter: there is no kind selector and no "pinned only" or "overridden only" toggle. A favorite authors list without authors has no filter field.

Tag completion in "Add pin" and "Add override":

27. The tag field of both forms offers suggestions while the user types, through one shared completion field. The search field of the local gallery listing and of the Panda listing uses the same shared field and behaves as before.
28. The suggestions are the tags of the Panda catalog, from the request of the Panda listing search (`GET /api/collector/catalog/completions` with the field's text and the cursor), matched as there. A short form works (`f:big` finds `female:big breasts`), as do a quoted value (`f:"big br`) and a value without a namespace (`glasses`).
29. The keys are those of the listing search: the arrow keys move through the suggestions, Enter chooses the marked one, Escape closes them, and a click chooses one.
30. While suggestions are open, Enter chooses one and does not submit the form. The suggestions are closed after that, so the next Enter submits the form when it can be submitted.
31. Choosing a suggestion replaces the whole text of the field with the plain tag `namespace:value` in the Panda catalog's spelling, without quotes and without `$`.
32. Saving a pin or an override takes a tag as a plain tag, as today (the namespace and the value in any case, `_` for a space), and now also with a short form for the namespace: `f:big breasts` saves the pin or the override of `female:big breasts`.
33. A tag pin and a tag override still have to name a tag that the Panda catalog has. A tag that it does not have answers "No gallery in the Panda catalog has this tag." in the form, also after a short form is expanded. Search syntax is not accepted on save: a value in quotes or with `$` is no tag of the catalog, and a value without a namespace answers "Enter a tag as namespace:value, an uploader name, or a Panda gallery category.", as today.
34. A failed completion request never interrupts typing, also while the collector is unreachable: the field shows no message and no suggestions, and works as a plain text field. A save then fails or succeeds as it does today.
35. The uploader field of both forms is a plain text field without suggestions. The category select stays.

## Design

No route, no migration and no package is created. Data flows as today: the browser calls the local service, whose handlers in `internal/local/httpapi` call the collector through `internal/collectorapi`, and the collector's handlers in `internal/collector/httpapi` call `internal/collector/catalog`. Two payloads change: the pins of a preview gain `model_weight` (ticket 03), and the `value` of a pin or an override accepts a short form (ticket 02). Paths, methods, status codes and error codes stay.

### Collector: `internal/collector/catalog`

**Short forms on save (ticket 02, `score_overrides.go`, `score_pins.go`).** `SetScorePin` and `SetScoreOverride` expand the namespace of a tag target after lowercasing it: a namespace that is a key of `gallerysearch.NamespaceShortForms` becomes that map's value. Everything else stays: the value rules of `parseScoreTarget`, the match by `(namespace, value_lower)` in `catalog_tags`, `ErrTagNotFound` for a tag the catalog does not have, and the label in the catalog's spelling. `ScoreTagWeights` does not expand: it matches the namespace as sent. Signatures do not change:

```go
func (s *Service) SetScorePin(ctx context.Context, kind, value string, weight float64) (collectorapi.ScorePin, error)
func (s *Service) SetScoreOverride(ctx context.Context, kind, value, rule string) error
func (s *Service) ScoreTagWeights(ctx context.Context, tags []string) (collectorapi.ScoreTagWeights, error)
```

The expansion is unconditional. The search language lets a namespace that exists under the very name of a short form win over the short form; Panda has no such namespace, so the save paths skip that lookup.

**Model weight of a pin (ticket 03, `score_model.go`).** `PreviewScoreModel` sets `ModelWeight` on each pin of `ScorePreview.Pins`: the weight that the candidate of this preview calculated for the pin's value, read from all weights of the candidate (the map that `scoreWeights` returns), keyed as `pinnedWeights` keys a pin (a tag by its `catalog_tags` ID, an uploader or category by its label). It is nil when the candidate has no weight for the value and when the preview has no candidate. The risky part is the source: `candidate.Weights` holds only the 500 strongest and must not be the source.

Test seam for both: the package's tests on PostgreSQL through `pgtest` (`score_pins_test.go`, `score_overrides_test.go`, `score_tag_weights_test.go`, `score_pins_preview_test.go`), with the helpers `openCatalog`, `retain`, `seedVerdicts`, `seedScoreEvidence`, `setPin` and `previewScore`.

### `internal/collectorapi` (`catalog_score.go`, ticket 03)

```go
// ScorePin is one pinned weight, labelled like an override. ModelWeight is set
// in a preview only: the weight the candidate calculated for the pin's value,
// nil without one.
type ScorePin struct {
	ID          int64    `json:"id"`
	Kind        string   `json:"kind"`
	Label       string   `json:"label"`
	Weight      float64  `json:"weight"`
	ModelWeight *float64 `json:"model_weight,omitempty"`
}
```

The answer of the pin route and the pin of a tag weight lookup leave the key out. The local service decodes and encodes `collectorapi.ScorePreview`, so the key reaches the browser without a change in `internal/local/httpapi`.

### HTTP

Unchanged routes, named here because the web calls them:

- `GET /api/collector/catalog/completions?q=<text>&cursor=<UTF-16 offset>` → `{"start","end","items":[{"namespace","value","term"}]}`. The forms use `namespace` and `value` and ignore `term`, which is search syntax (`female:"big breasts$"`).
- `POST /api/collector/catalog/score/pins` with `{"kind","value","weight"}` → the stored pin `{"id","kind","label","weight"}`. `POST /api/collector/catalog/score/overrides` with `{"kind","value","rule"}` → 204. Both answer 404 `tag_not_found` and 400 `invalid_pin` or `invalid_override` as today.
- `POST /api/collector/catalog/score/preview` → the preview, whose `pins` entries are `{"id","kind","label","weight","model_weight"?}`.

### Web: the shared tag completion field (tickets 01 and 07)

`web/src/galleries/TagCompletionField.tsx`, with its styles in `web/src/galleries/TagCompletionField.css`:

```ts
interface TagCompletionFieldProps {
  /** The input's id. The suggestion list is `<id>-suggestions`, its options `<id>-suggestion-<index>`. */
  id: string
  value: string
  onChange: (value: string) => void
  /** Requests the suggestions for the text and the cursor (a UTF-16 offset). */
  complete: (query: string, cursor: number, signal?: AbortSignal) => Promise<SearchCompletion>
  /** What a chosen suggestion puts in the field. Ticket 07; default 'term'. */
  insert?: 'term' | 'tag'
  type?: 'text' | 'search'
  placeholder?: string
  'aria-label'?: string
  /** `TextField.Slot` content, such as the search icon. */
  children?: ReactNode
}
export default function TagCompletionField(props: TagCompletionFieldProps): ReactNode
```

- The field is the `TextField.Root` with `role="combobox"` and the suggestion list that `GallerySearch` renders today, with everything that belongs to them: the cursor, focus, dismissal and IME composition state, the request 150 ms after the last change (only while focused, not dismissed, not composing, with a text that is not blank and a cursor above 0), the answer that is shown only for the text and cursor it was requested for, the arrow keys, Enter, Escape, the click on an option, and the cursor position after a choice. A failed request is ignored. The list keeps its accessible name "Tag suggestions".
- The caller owns the text (`value`, `onChange`) and the form. The field never submits and never navigates. Enter while suggestions are open chooses and is consumed. Enter while none are open dismisses them, so that a request still pending opens none, until the user types or clicks in the field; the key itself is left to the form. During IME composition the field handles no key, as today. A click on the form's button needs no rule: the field loses the focus, which closes the suggestions and stops a pending request.
- `insert: 'term'` replaces the completed range `[start, end)` of the text with the suggestion's `term` and puts the cursor behind it, as today. `insert: 'tag'` replaces the whole text with `` `${namespace}:${value}` `` and puts the cursor at its end.
- The style rules of the field's wrapper and of the suggestion list move from `Galleries.css` (`.gallery-search-input`, `.gallery-tag-suggestions` and its `li` rules) into `TagCompletionField.css`, which the component imports. The rules of the search form (`.gallery-search`) stay in `Galleries.css`.
- `GallerySearch.tsx` keeps its props and renders `<form role="search">` with the field (`id="gallery-search"`, `type="search"`, `aria-label="Search titles and tags"`, the placeholder, the search icon as a slot, `insert` left at its default) and the "Search" button. It owns the query text and navigates on submit. It still ignores a submit during IME composition, which `GallerySearch.test.tsx` pins; composition events bubble to the form.

Test seams: `web/src/galleries/GallerySearch.test.tsx` (unedited) for the search behaviour, and a new `web/src/galleries/TagCompletionField.test.tsx` that renders the field with a stubbed `complete` for what is the field's own (distinct ids, `insert: 'tag'`).

### Web: the score page (`web/src/panda/PandaScorePage.tsx`, `web/src/panda/score.ts`)

The texts in quotation marks are fixed. The page keeps its one table: the two other lists stay `<ul>` lists.

**Single-row layout (tickets 04 and 05).** A new stylesheet `web/src/panda/PandaScorePage.css`, imported by the page beside `PandaCatalog.css`, holds the rules:

- `.score-label` marks the label element of a row in all three lists: one line, hidden overflow, an ellipsis. Every label element also gets `title` with its full label.
- `.score-rows` marks the `<ul>` of the favorite authors list and of the "Pinned weights" list: it is the frame that scrolls sideways, each `<li>` is one line that does not wrap, the label takes the free width, and the other cells keep their content width. The cells line up as columns from row to row, so every row of a list has the same cells: a cell that a row has nothing to show in stays in the row.
- The custom property `--score-label-min-width: 12rem` is the least width of a label in all three lists. Below it the list scrolls inside its frame.
- In the weights table every cell stays on one line, and the label cell takes the free width with the same least width. The table keeps Radix's `Table.Root`, which scrolls sideways inside its frame.
- The override buttons keep their texts "Always hide" and "Never hide" and stop wrapping.

**Pin controls (ticket 04).** `PinControls` keeps its props, its field ("Pinned weight of <label>", `step="any"`, 6rem wide) and its rule for the field's text. Its three buttons become Radix `IconButton`s of size 1 that keep the accessible names and get titles: "Pin" (`DrawingPinIcon`, title "Pin the entered weight", disabled while the field holds no number), "Ignore" (`ValueNoneIcon`, title "Ignore: pin the weight 0") and, for a pinned value, "Remove pin" (`Cross2Icon`, title "Remove pin"). The eye icons are not used: the cards of the listing use them for overrides. The controls do not wrap and take the same width in every row: they start at the start of their cell, in the order field, "Pin", "Ignore", "Remove pin", and the place of "Remove pin" keeps its width in a row without a pin. So the fields of all rows stand in one column, and pinning a row neither shifts it nor narrows the labels. A row without a pin still has no button named "Remove pin". The "Pin" column header of the table starts at the start of its cell too, above the fields. `GalleryDetail.tsx` has its own pin editor and does not change.

**"Pinned weights" list (ticket 05).** `score.ts`: `ScorePin` gains `model_weight?: number`, which only the pins of a preview carry. `PinnedWeights` gets the pins of the last preview (`preview.pins`) beside the pin state that it lists. Each row of the list is an editor of its pin, and every row has the same five cells in this order:

- the label (`weightLabel(pin)`, `.score-label`, `title`);
- a number field named "Weight of pin <label>" (`step="any"`, 6rem wide) that starts with the pinned weight and follows the pin by the rule of `PinControls`: the text is set again only when the pin's id or weight changes;
- the place of the save control, an `IconButton` named "Save" (`CheckIcon`, title "Save the weight"). The button is rendered only while the field's text differs from the stored weight, as behaviour 14 defines it, and is disabled while the text is no number. The place keeps its width without the button, so the row does not shift when it shows;
- the model weight cell. The model weight of a row is the `model_weight` of the preview's pin with the row's kind and label, matched as `findPin` matches. With one, the cell holds "Model <weight>", formatted like the weights of the table (`formatWeight`), and the title "Weight in the new model". Without one the cell stays in the row and is empty, so that "Remove" stands in one column;
- the button "Remove", as today.

Enter in the field and the save control save through the page's pin change, as action `pin-list`, with the pin's kind, its label as the value and the field's number, only when the text is a number that differs from the stored weight and no change is being saved. Blur does nothing. "Save" and "Remove" are disabled while a change is being saved, as "Remove" is today; the field stays editable. `savePin`, its updater and the pin state do not change: a saved pin has the kind and label of the pin it replaces, so its row finds the same model weight. Messages stay where they are (`ChangeResult` for `pin-list` below the list).

**Filters (ticket 06).** `Weights` and `FavoriteAuthors` each own a filter text and the number of shown entries; `shown` and `onShown` leave `PandaScorePage` and `PreviewProps`. Setting the filter text sets the number of shown entries back to 50.

- The fields are text fields with the accessible names "Filter weights" (placeholder "Filter by tag, uploader or category") and "Filter favorite authors" (placeholder "Filter by artist"), above the count line of their list.
- An entry matches when its label, lowercased, contains the trimmed, lowercased filter text. The label of a weight is `weightLabel(weight)`, so "uploader" finds every uploader row. The label of an author is its tag.
- The count lines, the empty messages and the scope sentence are those of behaviours 23 to 25, with numbers through `toLocaleString()`. "<all>" is `candidate.weight_count` or `preview.favorite_author_count`, and "<n>" is the length of the list that the preview carries. The scope sentence follows the count line or the empty message in the same paragraph. With an empty result the table or the list is not rendered.
- "Show more" shows while more matches exist than are shown.
- The favorite authors list gets its count line also without a filter text. The filter field and the count line show only while the preview lists at least one author.

**Forms (ticket 07).** `PinForm` and `OverrideForm` render `TagCompletionField` for the kind "Tag" (`id` as today: `panda-score-pin-value`, `panda-score-override-value`; `value` and `onChange` are the form's text; `complete={completePandaSearch}` from `./api`; `insert="tag"`), under the same label "Tag (namespace:value)". For the kind "Uploader" they keep the plain `TextField.Root`, and for "Category" the select. Submit, the trimmed value, the disabled rule of the buttons and the messages do not change.

Test seam for the page: `web/src/panda/PandaScorePage.test.tsx`, which renders `<App />` at `#/panda/score` with a stubbed `fetch`. Layout is not tested (jsdom has none).

## Assumptions

- **The filter matches the label as shown**, with its "Uploader " or "Category " prefix. Rejected: matching the bare value. What the user reads is what matches, and "uploader" lists the uploader rows without a kind selector.
- **The filter ignores spaces around its text**, and a text of spaces only is no filter. Rejected: matching the text as typed, where a trailing space while typing "big breasts" would hide rows.
- **The filter runs in the browser over the entries of the preview.** Rejected: a filter on the collector over all weights (Fit 2). The scope sentence of behaviour 25 is added so that "No weights match." does not claim more than was searched.
- **The favorite authors list says "No favorite authors match."** The brief says that an empty result says that no weights match; an entry of this list is an author.
- **The favorite authors list gets a count line** ("<shown> of <all> favorite authors, most favorites first."), also without a filter text. The brief asks for a count line that says how many entries match, and the list has none today.
- **The model weight of a pin comes from the collector** as `model_weight` on the pins of a preview. Rejected: looking it up in the 500 weights of the preview, which misses most pinned values (Fit 2).
- **`model_weight` is an optional field of `collectorapi.ScorePin`**, set in previews only. Rejected: a second pin type for previews. It changes the type of `ScorePreview.Pins` for one number.
- **The page reads a pin's model weight from the pins of the last preview**, by kind and label, and not from its pin state (Fit 3). Rejected: copying the model weight into the pin state on save, which changes the `savePin` updater that all pin controls share. Also rejected: a pin route that looks up the candidate's weight. It would read `panda_score_models`, which the route avoids on purpose so that pins never wait for a calculation.
- **The row shows the model weight as "Model +0.50", and an empty cell without one.** Rejected: a dash. The brief shows it "when it has one". The cell stays in the row so that "Remove" keeps its column.
- **"Differs from the stored weight" compares numbers**, so "2.0" does not differ from 2. Rejected: comparing texts, where the save button would stay after saving "2.0". A text that is no number shows the disabled button, so the user sees that the field is not saved.
- **Enter in an unchanged field sends nothing.** Rejected: saving the same weight again, which starts a calculation for no change.
- **The save control is an icon button "Save"; "Remove" stays a text button.** Rejected: a text button "Save", which makes the row wider each time it shows.
- **The field of the list is named "Weight of pin <label>"**, not "Pinned weight of <label>" like a row's field (Fit 7).
- **The pin buttons keep the names "Pin", "Ignore" and "Remove pin"**, with the icons `DrawingPinIcon`, `ValueNoneIcon` and `Cross2Icon` and the titles of behaviour 7. Rejected: `EyeNoneIcon` for "Ignore", which the cards use for the "always hide" override.
- **The place of "Remove pin" stays reserved in a row without a pin**, and the pin controls start at the start of their cell. Rejected: controls at the end of their cell with a "Remove pin" that takes width only when it shows. The fields would stand in two columns, and the first pinned row on screen would widen the column and narrow every label.
- **The override buttons of the table keep their texts.** The brief asks for icon buttons for pin, ignore and remove pin only. Rejected: icons for the overrides too.
- **A label is at least 12rem wide** before its list scrolls (`--score-label-min-width`). The brief gives no number.
- **A short form is always expanded on save**, without checking whether the Panda catalog has a namespace of that very name, and only on the two save paths (Fit 4). Rejected: expanding in `parseScoreTarget` for all three callers, which would make the detail view show and pin `female:x` for a local tag `f:x`.
- **Save accepts plain tags only.** Rejected: accepting search syntax, which the brief does not ask for.
- **The completion request is sent unchanged**, so a value of several words without quotes completes by its last word, as in the listing search (`f:big bre` suggests tags with a word that starts with "bre" in any namespace). Choosing still replaces the whole field. Rejected: quoting the value before the request, which is not "the same request".
- **A pin field of a row loses typed text when the filter hides the row.** The row leaves the page. Rejected: keeping the texts of hidden rows in page state.
- **The filter text also stays across the favorites switch**, as the number of shown rows does today.
- **Terms not in `CONTEXT.md`:** *score page*, *new model* (the candidate of earlier specs), *model weight*, *short form* and *plain tag* are used as defined under **Behaviour**. None is a new domain concept.

## Out of scope

- A kind selector, and "pinned only" or "overridden only" toggles.
- A filter, an "Ignore" button or another order for the "Pinned weights" list.
- The "Overrides" list: it is not one of the three lists and keeps its layout.
- Completion for uploaders, and any change to the category select.
- The pin popover of the gallery detail view.
- A filter over the weights or the authors that the preview does not carry, and any change to the two limits of 500.
- Changes to the completion itself: its matching, its limit of 10 suggestions and its delay.
- Search syntax on save, and short forms in the tag weight lookup of the detail view.
- Changes to `README.md` and `CONTEXT.md`.

## Manual checks

- **One line per entry.** Open the score page with a calculated model, some pins and favorite authors, in a wide window (1280 px and more), a narrow one (about 700 px) and at phone width (390 px). Good: no row of the three lists is taller than one line, a long label ends with an ellipsis, each list scrolls sideways inside its own frame, and the page has no horizontal scrollbar. The constant to tune is `--score-label-min-width` (12rem) in `PandaScorePage.css`: larger when labels are cut too early on a narrow window, smaller when the lists scroll too soon.
- **Columns.** Look down each list, then pin a row that has no pin, remove the pin again, and type in a field of the "Pinned weights" list. Good: the cells line up from row to row; the number fields stand in one column whether or not a row is pinned; "Remove" of the "Pinned weights" list stands in one column with and without a model weight; no row shifts and no label gets narrower when "Remove pin" or the save control shows. The width to tune is the field's 6rem.
- **Tooltip.** Hover a truncated label and a short one in each list. Good: both show the full label.
- **Icon buttons.** Good: pin, ignore and remove are easy to tell apart at size 1, each shows its title on hover, and on a touch screen they stay tappable.
- **Editing pins against the real collector.** Change a weight in the "Pinned weights" list and press Enter, then type in another pin field while the spinner runs. Good: the saved row shows its weight and its model weight at once, the typed text is still there when the calculation answers, and a pin made from the table shows its model weight about one calculation later.
- **Filter speed.** With 500 weights in the preview, type in "Filter weights". Good: typing stays fluid. The plan picks no delay; a delay before filtering is the fix if it does not.
- **Completion in the forms.** Against the real collector, type `f:big`, `f:"big br` and `glasses` in the tag field of "Add pin" and of "Add override". Good: the suggestions open below the field, above the content that follows, and are not cut off; they come as fast as in the listing search; Enter chooses, and the next Enter adds the pin once a weight is entered. The delay is the 150 ms that the shared field keeps from the gallery search.
- **Listing searches.** Search in the local gallery listing and in the Panda listing. Also type a few characters and press Enter at once, before suggestions open. Good: suggestions, keys and submit behave as before the shared field, and no suggestions open after Enter has started the search.
- **Collector unreachable.** Stop the collector and type in a tag field. Good: no message and no suggestions while typing, and the save reports the collector as it does today.
