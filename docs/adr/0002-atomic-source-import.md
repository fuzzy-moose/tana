# Atomic source import and automatic gallery creation

Scans discover only unregistered source paths, so a committed inventory without its intended automatic gallery would be skipped by later scans. Commit each source's complete inventory and automatic gallery, when applicable, in one transaction so a failed import remains eligible for retry without requiring gallery repair or durable scan history. Prepare inventories outside the transaction, allowing bounded parallel filesystem work while keeping database writes serialized and transactions short.
