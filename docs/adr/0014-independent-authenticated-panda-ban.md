---
status: accepted
---

# Isolate authenticated Panda bans

Authenticated Panda requests, including favorites and archive preparation, share one persisted ban independent of the main unauthenticated request group, so either group can continue while the other is banned. Changing cookies, account, or Panda host does not establish that the restriction has expired and therefore does not clear the authenticated ban. Archive file transfers are outside ban coordination: they neither observe nor establish a Panda ban.

The existing shared deadline carries forward only to the main unauthenticated group; the authenticated group starts without an inherited ban despite the stored deadline not identifying its originating request. Existing per-job retry deadlines remain untouched, so work already waiting may remain delayed by the former shared cooldown.

An authenticated ban does not interrupt active archive transfers. Transfer failures retain ordinary bounded retries, but obtaining a fresh archive link for a retry remains authenticated work and must respect that group's ban. Cooldown reporting identifies the affected group and its deadline, because neither group's ban means all Panda requests are paused.

Ban persistence follows [ADR 0008](0008-share-panda-ban-state.md), and archive ownership follows [ADR 0009](0009-collector-owns-panda-downloads.md). Metadata proxy channels retain their independent bans under [ADR 0012](0012-independent-metadata-proxy-bans.md).
