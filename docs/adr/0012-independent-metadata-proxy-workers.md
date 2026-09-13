---
status: accepted
---

# Isolate metadata proxy workers and share collection work

Optional Panda metadata proxy channels use independent metadata limiters and failure cooldowns so collection through separate proxies can pause and progress independently. Ban boundaries and persistence follow [ADR 0008](0008-panda-ban-groups.md). A proxy failure leaves gallery work available to healthy workers because it does not establish gallery unavailability.

The collector owns persistent proxy settings, changed through Tana's local UI, so every local client controls the same collection feature. Configuration changes let an assigned batch finish and save its result before taking effect, preserving completed work. Saved proxy passwords are write-only so one local client cannot retrieve passwords supplied by another.

Proxy channels prioritize eligible, unreserved inventory references, including sitemap backfill, and may validate imports when they cannot claim inventory work, even while other workers hold inventory reservations or explicit fetch jobs remain pending on the main fetcher. This keeps proxy capacity useful during inventory collection and main-fetcher cooldowns. Shared gallery reservations and result admission prevent duplicate concurrent requests across inventory and import work while all workers contribute to the same retained metadata. Main-fetcher priority follows [ADR 0006](0006-queue-explicit-metadata-fetches.md); inventory reconciliation follows [ADR 0011](0011-bulk-panda-reference-imports.md).
