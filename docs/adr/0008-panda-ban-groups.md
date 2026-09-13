---
status: accepted
---

# Persist Panda bans by request group

Requests within a Panda ban group share a persisted deadline so a ban prevents further requests through that group across restarts. Main unauthenticated metadata requests and sitemap collection share one ban; authenticated favorites and archive preparation share another, along with their session and request pacing. Each metadata proxy channel owns an independent ban so separate routes can pause and progress independently. Group isolation does not assume independent upstream rate allowances.

Changing cookies, account, or Panda host does not clear the authenticated ban because configuration changes do not establish that an upstream restriction has expired. Proxy bans likewise survive channel edits and deletion followed by recreation of the same proxy. Proxy identity is its normalized protocol, hostname, and port, independent of credentials, so different logins on one gateway share a ban identity.

Plain-text ban responses are recognized through keywords to tolerate wording changes; an extracted duration sets the cooldown with a small margin, and an unreadable duration imposes a conservative one-day cooldown. Unrelated successful requests cannot clear a ban. Favorite syncs preserve their checkpoints through cooldown, and reporting identifies the affected group and deadline because a ban does not pause all Panda work.

Feeds and archive file transfers remain outside ban coordination. Transfers use a separate client without Panda cookies because archive hosts do not need the account session; they neither observe nor establish a Panda ban, and an authenticated ban does not interrupt active transfers. Transfer failures retain bounded retries, but obtaining a fresh archive link remains authenticated work and respects that group's ban.
