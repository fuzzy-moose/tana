---
status: accepted
---

# Treat bulk Panda reference imports as background discovery

Bulk Panda reference imports reuse matching inventory references regardless of their metadata collection outcome and validate new references according to [ADR 0006](0006-queue-explicit-metadata-fetches.md), preserving inventory history across repeated submissions. The collector accepts responsibility for complete input files and retains outstanding work across restarts, so imports can finish independently of the submitting Tana session. Bulk validation runs after all other metadata collection in the main fetcher and after inventory collection, including sitemap backfill, in optional proxy channels. Proxy channels may validate imports while explicit jobs wait on the main fetcher, as decided in [ADR 0012](0012-independent-metadata-proxy-bans.md).
