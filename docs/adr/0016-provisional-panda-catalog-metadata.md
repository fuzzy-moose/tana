---
status: accepted
---

# Browse provisional Panda metadata while upstream changes settle

Newly published Panda galleries can change before their metadata settles. Feed descriptions make these galleries browsable during a configurable collection delay, trading initial completeness for earlier catalog availability and a more settled collected snapshot. Provisional metadata remains separate from collected metadata because applying a feed snapshot to local galleries would otherwise complete enrichment prematurely.

Collected metadata replaces the provisional description entirely, including tags: retaining feed-only values would preserve information that upstream corrections may have removed. Failed collection leaves the provisional description useful for browsing. Unknown expunged status remains visible so the default catalog view includes provisional entries.

The delay belongs to shared collector scheduling so rediscovery through another automatic path cannot circumvent it. Explicit requests intentionally bypass settling, and successful early retrieval satisfies collection under [ADR 0005](0005-separate-feed-sightings-from-gallery-inventory.md). Eligibility remains a lower bound, not a promised retrieval time, because collector availability and upstream pacing still apply.

Treat Panda's Atom `updated` value as its gallery publication time. This is a Panda-specific interpretation adopted for this design, rather than a claim about generic Atom semantics. First sighting provides the fallback without claiming a publication date. Earlier recovered sightings may shorten the wait; repeated captures must never postpone it. Historical processed feed bodies were discarded, so provisional descriptions require newly processed captures rather than an upstream backfill solely for this feature.
