---
status: accepted
---

# Isolate bans for optional metadata proxy channels

Optional Panda metadata proxy channels each own their upstream ban state and use an independent instance of the existing metadata limiter so collection through separate proxies can pause and progress independently. They assist background inventory collection and reference import validation while the main metadata fetcher retains its responsibilities and shared ban coordination with other Panda consumers. Shared work reservations and result admission prevent duplicate concurrent gallery requests while allowing both fetchers to contribute to the same retained metadata.

The collector owns persistent proxy settings, changed through Tana's local UI, so every local client controls the same collection feature. Configuration changes let an assigned batch finish and save its result before taking effect, preserving completed work. Saved proxy passwords are write-only so one local client cannot retrieve passwords supplied by another.

An active ban survives channel edits and deletion followed by recreation of the same proxy, because configuration changes do not establish that the upstream restriction has expired. Proxy identity is its normalized protocol, hostname and port, independent of credentials; different logins on one gateway therefore share an identity. A proxy failure leaves gallery work available to healthy workers because it does not establish gallery unavailability.

Proxy channels may validate imports once background inventory is drained even when explicit fetch jobs remain pending on the main fetcher. This keeps independent proxy capacity useful during main-fetcher cooldowns and narrows [ADR 0011](0011-bulk-panda-reference-imports.md)'s priority rule for proxy work; the main fetcher's ordering remains intact.

This narrows the shared-ban scope of [ADR 0008](0008-share-panda-ban-state.md) and its extension in [ADR 0009](0009-collector-owns-panda-downloads.md), and limits [ADR 0006](0006-queue-explicit-metadata-fetches.md)'s shared pacing and failure cooldown to the main fetcher. Independent upstream rate allowances have not been established by the published API guidance.
