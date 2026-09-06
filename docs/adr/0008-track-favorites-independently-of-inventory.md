---
status: accepted
---

# Track favorites independently of the gallery reference inventory

Favorite collection state belongs to a Panda host, account, and category, independently of gallery references discovered through other paths, because shared inventory membership cannot establish how far favorites have been collected. Dedicated favorite records retain gallery references and favorite timestamps; incremental collection relies on the configured profile presenting newest favorites first in pages of 100, while full re-sync reconciles membership without deleting shared inventory or metadata. Category state and discovered inventory references commit together only after a successful traversal, allowing interrupted in-memory jobs to be discarded without advancing the collection boundary past missing entries.
