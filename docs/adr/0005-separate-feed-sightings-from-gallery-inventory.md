# Separate feed sightings from the gallery reference inventory

Feed gallery references record sightings uniquely by raw feed and gallery ID, while a separate gallery reference inventory holds galleries discovered through either feeds or Panda metadata uniquely by gallery ID. Feed continuity depends on feed sightings because API discovery does not establish that a gallery appeared in a captured feed. Metadata collection uses the shared inventory so references discovered through either path are fetched once after success.
