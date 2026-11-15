# 04: One line per row and icon pin controls in the weights table and the favorite authors list

What to build: on the score page, every row of the weights table and every row of the favorite authors list stays on one line. The label takes the width that the other cells leave free, ends with an ellipsis when it is too long, and shows its full text on hover. The pin controls of both lists become a number field with small icon buttons, and Enter in the field pins. When the window is too narrow, each list scrolls sideways inside its own frame, and the page does not.

This is spec behaviours 1 to 10 for these two lists (ticket 05 does the "Pinned weights" list), specified in spec **Design → Web: the score page → Single-row layout** and **Pin controls**, and **Fit** 7 and 9.

- **`web/src/panda/PandaScorePage.css`** (new, imported by the page beside `PandaCatalog.css`):
  - `.score-label`: the label element of a row. One line, hidden overflow, an ellipsis.
  - `.score-rows`: the `<ul>` of a list. It is the frame that scrolls sideways. Each `<li>` is one line that does not wrap, the label takes the free width, the other cells keep their content width, and the cells line up as columns from row to row. Every row of a list has the same cells: a cell that a row has nothing to show in stays in the row.
  - `--score-label-min-width: 12rem`: the least width of a label, in the table too. Below it the list scrolls inside its frame.
  - Ticket 05 uses `.score-rows` and `.score-label` for the "Pinned weights" list.
- **Weights table** (`Weights`): every cell stays on one line. The label of a row is its `.score-label`, on the cell or on an element inside it. It takes the free width and carries the full label (`weightLabel(weight)`) as `title`. The table keeps Radix's `Table.Root`, whose scroll area scrolls sideways. Trap: a table cell that does not wrap grows to its content, so the label cell does not shrink to an ellipsis by itself.
- **Override buttons** (`OverrideButtons`): the texts stay, the two buttons no longer wrap.
- **Pin controls** (`PinControls`): the props, the field (named "Pinned weight of <label>", `step="any"`, 6rem wide) and the rule for the field's text stay. The buttons become `IconButton`s of size 1 with an icon and no text, in this order after the field:
  - "Pin": `DrawingPinIcon`, title "Pin the entered weight", disabled while the field holds no number;
  - "Ignore": `ValueNoneIcon`, title "Ignore: pin the weight 0";
  - "Remove pin", only for a pinned value: `Cross2Icon`, title "Remove pin".

  The accessible names are the `aria-label`s "Pin", "Ignore" and "Remove pin", as the existing tests find them. The controls do not wrap and take the same width in every row: they start at the start of their cell, and the place of "Remove pin" keeps its width in a row without a pin. Such a row still has no button named "Remove pin". The fields of all rows then stand in one column, and pinning a row neither shifts it nor narrows the labels. The "Pin" column header of the table starts at the start of its cell too, above the fields.
- **Favorite authors list** (`FavoriteAuthors`): the `<ul>` becomes `.score-rows`. A row keeps its three cells with their texts (label, "<n> favorites", weight) and their marker `data-author-cell`, which `listedAuthors()` of the tests reads, followed by the pin controls. The label cell is the `.score-label` and carries the author's tag as `title`.
- `web/src/galleries/GalleryDetail.tsx` has its own pin editor and does not change.

Test seam: `web/src/panda/PandaScorePage.test.tsx` with a stubbed `fetch`. Its assertions on "Pin", "Ignore", "Remove pin", the override buttons, `rows()` and `listedAuthors()` stay unedited. Layout is not tested (jsdom has none): that no row wraps, the ellipsis, the scrolling frame and the page's width are manual checks (spec **Manual checks**, "One line per entry", "Columns", "Tooltip", "Icon buttons"). If you want to look at the result, the `run-tana` skill starts the app; the score page shows a weights table once some catalog galleries have verdicts of both kinds.

Blocked by: None

Status: resolved

- [x] In the weights table, the label of each row carries its full label as `title` ("female:glasses", "Uploader Some uploader", "Category Doujinshi"). In the favorite authors list, the label of each author carries the author's tag as `title`.
- [x] The pin controls of a row in the weights table and in the favorite authors list are the number field "Pinned weight of <label>" and the buttons "Pin", "Ignore" and, only for a pinned value, "Remove pin": a row without a pin has no button of that name. Each button has no text content and carries its title: "Pin the entered weight", "Ignore: pin the weight 0", "Remove pin".
- [x] Enter in a row's pin field that holds a number posts the row's kind, its label as the value and the number to `/api/collector/catalog/score/pins`. Enter in a field without a number sends nothing.
- [x] The buttons act as before: the existing assertions of `PandaScorePage.test.tsx` on pin changes from the table and from the favorite authors list, on the override buttons, on `rows()` and on `listedAuthors()` pass unedited.
- [x] The pin popover of the gallery detail view is as before: `GalleryDetail.test.tsx` passes unedited.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`.

## Resolution

New `web/src/panda/PandaScorePage.css` (`.score-label`, `.score-rows` subgrid frame sized by `--score-columns`, `.score-weights` table rules, `.score-pin-slot`); `PandaScorePage.tsx` gives labels a `title`, makes `PinControls` icon buttons that start at their cell with a reserved "Remove pin" slot, and turns the favorite authors list into `.score-rows`; tests cover titles, icon buttons and Enter.
