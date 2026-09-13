---
status: accepted
---

# Isolate bans for optional metadata proxy channels

Optional Panda metadata proxy channels each own their upstream ban state and use an independent instance of the existing metadata limiter so collection through separate proxies can pause and progress independently. They assist background inventory collection and reference import validation while the main metadata fetcher retains its responsibilities and shares ban coordination with sitemap collection. Shared work reservations and result admission prevent duplicate concurrent gallery requests while allowing both fetchers to contribute to the same retained metadata.

The collector owns persistent proxy settings, changed through Tana's local UI, so every local client controls the same collection feature. Configuration changes let an assigned batch finish and save its result before taking effect, preserving completed work. Saved proxy passwords are write-only so one local client cannot retrieve passwords supplied by another.

An active ban survives channel edits and deletion followed by recreation of the same proxy, because configuration changes do not establish that the upstream restriction has expired. Proxy identity is its normalized protocol, hostname and port, independent of credentials; different logins on one gateway therefore share an identity. A proxy failure leaves gallery work available to healthy workers because it does not establish gallery unavailability.

Proxy channels prioritize eligible, unreserved inventory references and may validate imports when they cannot claim inventory work, even while other workers hold inventory reservations or explicit fetch jobs remain pending on the main fetcher. This keeps independent proxy capacity useful during inventory collection and main-fetcher cooldowns. Shared gallery reservations apply to both inventory and import work; the main fetcher retains its explicit-job priority. [ADR 0011](0011-bulk-panda-reference-imports.md) also requires pending imports to reconcile with inventory independently of this scheduling, so inventory matches can settle without waiting for validation capacity.

The shared pacing and failure cooldown in [ADR 0006](0006-queue-explicit-metadata-fetches.md) apply to the main fetcher. Authenticated requests own a separate ban under [ADR 0014](0014-independent-authenticated-panda-ban.md). Independent upstream rate allowances have not been established by the published API guidance.
