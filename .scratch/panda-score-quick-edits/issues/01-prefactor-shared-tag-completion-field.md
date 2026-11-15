# 01: Prefactor: The gallery search takes its suggestions from a shared tag completion field

What to build: nothing the user sees changes. The text field with tag suggestions moves out of `web/src/galleries/GallerySearch.tsx` into a component of its own, so that ticket 07 can put the same field into the "Add pin" and "Add override" forms of the score page (spec **Fit** 1 and **Design → Web: the shared tag completion field**).

- **`web/src/galleries/TagCompletionField.tsx`** holds the `TextField.Root` with `role="combobox"` and the suggestion list, with all the state and handling that belong to them today: cursor, focus, dismissal, IME composition, the request 150 ms after the last change, the answer shown only for the text and cursor it was requested for, the arrow keys, Enter, Escape, the click on an option, and the cursor after a choice. Its props are those of the spec without `insert`, which ticket 07 adds: `id`, `value`, `onChange`, `complete`, `type`, `placeholder`, `aria-label` and `children` (the `TextField.Slot`).
- **The caller owns the text and the form.** The field never submits and never navigates. Enter while suggestions are open chooses one. Enter while none are open dismisses them, so that a request still pending opens none, until the user types or clicks in the field; the key itself is left to the form. During IME composition the field handles no key, as today. A click on the form's button needs no rule: the field loses the focus, which closes the suggestions and stops a pending request.
- **Ids come from the `id` prop**: the suggestion list is `<id>-suggestions` and its options are `<id>-suggestion-<index>`, so two fields can be on one page. The list keeps the accessible name "Tag suggestions".
- **`web/src/galleries/TagCompletionField.css`**, imported by the component, takes the rules of the field's wrapper and of the suggestion list from `Galleries.css` (`.gallery-search-input`, `.gallery-tag-suggestions` and its `li` rules). The rules of the form (`.gallery-search`) stay in `Galleries.css`. Class names are yours.
- **`GallerySearch.tsx`** keeps its props (`search`, `completeSearch`, `searchHref`) and renders the form with the field (`id="gallery-search"`, `type="search"`, `aria-label="Search titles and tags"`, the placeholder, the search icon) and the "Search" button. It owns the query text and navigates on submit. It still ignores a submit during IME composition: `GallerySearch.test.tsx` fires a submit between `compositionStart` and `compositionEnd` and expects no navigation. Composition events bubble to the form.

Test seams: `web/src/galleries/GallerySearch.test.tsx` covers the search behaviour and stays unedited. A new `web/src/galleries/TagCompletionField.test.tsx` renders the field with a stubbed `complete` for the one thing that is new, the ids. No existing test file changes.

Blocked by: None

Status: resolved

- [x] The search field of the local gallery listing and of the Panda listing behaves as before: `GallerySearch.test.tsx`, `Galleries.test.tsx`, `PandaCatalog.test.tsx` and `App.test.tsx` pass unedited.
- [x] `TagCompletionField` rendered alone, with a `value`, an `onChange` and a stubbed `complete`, opens the list "Tag suggestions" for a typed text, and choosing a suggestion calls `onChange` with the text in which the suggestion's `term` replaces the completed range. It changes no location hash.
- [x] Two `TagCompletionField`s with different `id`s rendered together keep their suggestions apart: the `aria-controls` of the field with open suggestions is `<id>-suggestions`, and the document holds one element with that id.
- [x] `npm test`, `npm run lint` and `npm run build` pass in `web/`, and no existing test file changes.

## Resolution

The tag completion field moved from `GallerySearch.tsx` into `web/src/galleries/TagCompletionField.tsx` (ids from `id`, Enter without open suggestions dismisses them) with its styles in `TagCompletionField.css`; `GallerySearch` now renders the form around it and tracks composition on the form; `TagCompletionField.test.tsx` covers choosing a suggestion and distinct ids.
