# Queue explicit metadata fetches separately from the inventory

Explicit metadata fetch jobs persist supplied references before Panda validates them so requests survive restarts, but only successful validation admits those references to the gallery reference inventory. Jobs take priority over feed-discovered collection while sharing upstream pacing and failure cooldown, keeping Tana's requested work responsive without bypassing upstream limits. Stored reads remain independent of retrieval, allowing retained metadata to stay available while fresh retrieval is pending or fails.
