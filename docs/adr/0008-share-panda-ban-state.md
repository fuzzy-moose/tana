---
status: accepted
---

# Share Panda ban state across metadata and favorites

Favorites and metadata retain separate request pacing, with favorites limited to one request per ten seconds, but share a persisted ban deadline because a ban encountered through either path must prevent further requests through both, including after restart. Plain-text ban responses are recognized through keywords rather than an exact message; an extracted duration sets the cooldown with a small margin, and an unreadable duration imposes a one-day cooldown. Feeds remain outside this coordination; unrelated successful requests cannot clear the ban. Favorite syncs retain their checkpoints throughout the cooldown and resume after it expires.
