---
status: accepted
---

# Treat bulk Panda reference imports as background discovery

Bulk Panda reference imports reuse matching inventory references regardless of their metadata collection outcome and validate new references according to [ADR 0006](0006-queue-explicit-metadata-fetches.md), preserving inventory history across repeated submissions. The collector accepts responsibility for complete input files and retains outstanding work across restarts, so imports can finish independently of the submitting Tana session. Bulk validation runs after all other metadata collection, including sitemap backfill, accepting delayed completion to keep large file submissions from displacing existing collection.
