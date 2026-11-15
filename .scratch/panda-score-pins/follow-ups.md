# Follow-ups

- Do the manual visual checks from the spec: pin and star icons at card size, the detail view's popover position, and the ghost-button margin reset in GalleryDetail.css.
- Optionally extract a shared local-collector test fixture (gallery_ref insert, projection, collector handler behind httptest) for all of internal/local/httpapi/catalog_test.go, not just the new tests.
- Optionally pull out a shared pin field-and-buttons component for GalleryDetail's TagWeight and PandaScorePage's PinControls if the two editors' behaviour converges.
