# 06: Filters for the weights table and the favorite authors list

What to build: on the score page, a text field above the weights table and one above the favorite authors list filter their list by part of the label, ignoring case. The filter searches every entry that the preview carries, not only the 50 that are shown, and the list then pages through the matches. The count line says how many entries match, and an empty result says so. The filter text stays through a recalculation, and a new filter text starts the list again at its first 50 matches.

This is spec behaviours 19 to 26, specified in spec **Design → Web: the score page → Filters** and **Fit** 2 and 5.

- `web/src/panda/PandaScorePage.tsx`: `Weights` and `FavoriteAuthors` each own their filter text and their number of shown entries. `shown` and `onShown` leave `PandaScorePage` and `PreviewProps`: both components stay mounted from one calculation to the next, which keeps both values. Setting the filter text sets the number of shown entries back to 50.
- The fields: text fields with the accessible names "Filter weights" (placeholder "Filter by tag, uploader or category") and "Filter favorite authors" (placeholder "Filter by artist"), above the count line of their list.
- Matching: an entry matches when its label, lowercased, contains the trimmed, lowercased filter text. The label of a weight is `weightLabel(weight)`, so "uploader" finds every uploader row. The label of an author is its tag. A text that is empty after trimming is no filter.
- The lists show the first 50 matches, "Show more" adds 50 and shows while more matches exist than are shown.
- The texts, with numbers through `toLocaleString()`. "<all>" is `candidate.weight_count` or `preview.favorite_author_count`, and "<n>" is the length of the list that the preview carries (`candidate.weights`, `preview.favorite_authors`):

  | List | State | Text |
  | --- | --- | --- |
  | Weights | no filter text | "<shown> of <all> weights, strongest first." (as today) |
  | Weights | filter text, matches | "<shown> of <matches> matching weights, strongest first." |
  | Weights | filter text, no match | "No weights match." in place of the table |
  | Weights | filter text and <n> below <all> | the line or the message, followed in the same paragraph by " Only the <n> strongest of <all> weights are searched." |
  | Authors | no filter text | "<shown> of <all> favorite authors, most favorites first." (new) |
  | Authors | filter text, matches | "<shown> of <matches> matching favorite authors, most favorites first." |
  | Authors | filter text, no match | "No favorite authors match." in place of the list |
  | Authors | filter text and <n> below <all> | the line or the message, followed in the same paragraph by " Only the <n> authors with the most favorites of <all> are searched." |

- The filter field and the count line of the favorite authors list show only while the preview lists at least one author. The sentence "<count> favorite authors: artist tags that at least 2 Panda favorites carry." stays.
- There is no kind selector and no toggle. The "Pinned weights" list gets no filter.
- A pin field of a row that the filter hides loses its typed text, because the row leaves the page. This is accepted (spec **Assumptions**).

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`, and its generators `weight(index)` and `author(index)`. The existing tests of the paging and of "the shown weights stay shown after an override calculates again" stay unedited.

Blocked by: None

Status: resolved

- [x] Typing in "Filter weights" leaves the rows whose label contains the text, ignoring case and spaces around the text, among all weights of the preview: a row beyond the first 50 shows when it matches, and "uploader" shows the row "Uploader Some uploader".
- [x] With a filter text the count line of the weights table reads "<shown> of <matches> matching weights, strongest first.", and without one it reads as before.
- [x] A filter text that no weight matches shows "No weights match." and no table.
- [x] With more than 50 matches the table shows 50, "Show more" adds 50 matches, and changing the filter text shows 50 again.
- [x] After a pin change or an override change calculates again, "Filter weights" still holds its text, and the table shows the same number of rows as before the change.
- [x] With a filter text and a preview that carries fewer weights than `weight_count`, the count line and the empty message end with "Only the <n> strongest of <all> weights are searched." Without a filter text, or when the preview carries all weights, they do not.
- [x] "Filter favorite authors" filters the list by the author's tag in the same way: matches beyond the first 50 show, the count line reads "<shown> of <matches> matching favorite authors, most favorites first.", no match shows "No favorite authors match." and no list, a changed text shows the first 50 matches again, and the text stays after a pin change calculates again.
- [x] Without a filter text the favorite authors list shows "<shown> of <all> favorite authors, most favorites first.", and with a filter text and fewer listed authors than `favorite_author_count` the line ends with "Only the <n> authors with the most favorites of <all> are searched." A preview without authors shows no filter field and no count line.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, and the existing paging tests pass unedited.

## Resolution

`PandaScorePage.tsx`: a `useFilteredList` hook gives `Weights` and `FavoriteAuthors` their own filter text and shown count (`shown`/`onShown` removed from the page and `PreviewProps`), with `FilterField` and `CountLine` rendering the fields, count lines, empty messages and scope sentence; covered by new tests in `PandaScorePage.test.tsx`.
