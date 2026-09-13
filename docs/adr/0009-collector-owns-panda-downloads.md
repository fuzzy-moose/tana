# Collector owns Panda downloads

The collector owns durable Panda download jobs and retains completed original archives until explicit deletion, keeping upstream access and archive availability independent of Tana library availability. Retrieving an archive does not consume it or establish local library presence, so delivery can be retried without downloading from Panda again.

Favorites and archive preparation share one authenticated session, request pacing, and persisted ban under [ADR 0014](0014-independent-authenticated-panda-ban.md). Archive file transfers use a separate client without Panda cookies because the upstream archive hosts do not need the account session, and remain outside ban coordination.
