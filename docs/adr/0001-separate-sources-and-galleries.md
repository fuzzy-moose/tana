# Separate sources and galleries

Sources inventory all contained files, while galleries select and order supported images through independently numbered page occurrences. Galleries are independent of any single library so they can combine sources across libraries, reuse a subset of a source, and repeat a file without duplicating its inventory entry.

Automatically created source-linked galleries have a lifecycle tied to their single source: their page membership remains source-derived, while page order and gallery metadata are independent. Explicit source or library deletion deletes these linked galleries; independent galleries lose affected pages, renumber their remaining pages, and survive even when empty to preserve their independent identity. Temporary library unavailability does not remove inventory, galleries, or pages.
