---
status: accepted
---

# Persist Panda bans within request groups

Requests within a Panda ban group share a persisted deadline because a ban must prevent further requests through that group, including after restart. Plain-text ban responses are recognized through keywords rather than an exact message; an extracted duration sets the cooldown with a small margin, and an unreadable duration imposes a one-day cooldown. Feeds remain outside this coordination; unrelated successful requests cannot clear the ban. Favorite syncs retain their checkpoints throughout the cooldown and resume after it expires. Authenticated requests own an independent ban under [ADR 0014](0014-independent-authenticated-panda-ban.md); optional metadata proxy channels own independent bans under [ADR 0012](0012-independent-metadata-proxy-bans.md).
