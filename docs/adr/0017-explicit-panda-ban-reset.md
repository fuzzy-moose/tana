# Allow explicit Panda ban resets

A user can reset the main, authenticated, or proxy ban group through Tana's local UI, because a persisted ban can be recorded in error and otherwise blocks that group until its deadline. This refines [ADR 0008](0008-panda-ban-groups.md): configuration changes and unrelated successful requests still never clear a ban, since neither establishes that an upstream restriction has ended, while an explicit reset expresses the user's judgment that it has.

A reset also ends retry waits that recorded the ban as their deadline, so blocked work resumes without waiting for the original cooldown; failures with ordinary backoff keep it. The proxy group resets every channel and endpoint together, matching the scope of a false detection that follows one batch across channels. A ban still in force upstream is recorded again by the next request through that group.
