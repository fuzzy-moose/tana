# Separate feed sightings from the gallery reference inventory

Feed gallery references record sightings uniquely by raw feed and gallery ID, while a separate gallery reference inventory consolidates all discovery paths uniquely by gallery ID. Feed continuity depends only on feed sightings because discovery elsewhere does not establish that a gallery appeared in a captured feed. The shared inventory prevents repeated discovery from scheduling metadata collection again after success; explicit refresh requests remain separate under [ADR 0006](0006-queue-explicit-metadata-fetches.md).
