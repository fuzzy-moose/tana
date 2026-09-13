---
status: accepted
---

# Treat bulk Panda reference imports as background discovery

Bulk Panda reference imports reuse matching inventory references regardless of their metadata collection outcome and validate new references according to [ADR 0006](0006-queue-explicit-metadata-fetches.md), preserving inventory history across repeated submissions. The collector accepts responsibility for complete input files and retains outstanding work across restarts, so imports can finish independently of the submitting Tana session.

Bulk validation runs after all other metadata collection in the main fetcher so bulk submissions do not take priority over ongoing collection. Proxy scheduling follows [ADR 0012](0012-independent-metadata-proxy-workers.md).

Pending imports reconcile with the gallery reference inventory independently of metadata scheduling: matching tokens are already known and conflicting tokens fail. Bounded, indexed reconciliation keeps continuous inventory collection from delaying outcomes that require no upstream validation, while limiting database write work for large imports.
