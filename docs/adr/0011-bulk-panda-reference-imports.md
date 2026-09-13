---
status: accepted
---

# Treat bulk Panda reference imports as background discovery

Bulk Panda reference imports reuse matching inventory references regardless of their metadata collection outcome and validate new references according to [ADR 0006](0006-queue-explicit-metadata-fetches.md), preserving inventory history across repeated submissions. The collector accepts responsibility for complete input files and retains outstanding work across restarts, so imports can finish independently of the submitting Tana session.

Bulk validation runs after all other metadata collection in the main fetcher. Optional proxy channels prioritize eligible, unreserved inventory references, including sitemap backfill, and validate imports when they cannot claim inventory work. Shared gallery reservations preserve exclusive ownership while spare proxy capacity progresses imports, including while explicit jobs wait on the main fetcher, as decided in [ADR 0012](0012-independent-metadata-proxy-bans.md).

Pending imports reconcile with the gallery reference inventory independently of metadata scheduling: matching tokens are already known and conflicting tokens fail. Bounded, indexed reconciliation keeps continuous inventory collection from delaying outcomes that require no upstream validation, while limiting database write work for large imports.
