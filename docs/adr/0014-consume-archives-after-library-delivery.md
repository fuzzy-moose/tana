---
status: accepted
---

# Consume collector archives after library delivery

Explicit delivery into a Tana library transfers responsibility for the archive to Tana: remove the collector copy only after the destination file is safely saved and its source imported. This makes library delivery responsible for collector cleanup while preserving a recoverable collector copy when saving or importing fails; an import retry reuses the saved local file.

Delivery progress survives browser closure and Tana restarts because ownership handoff spans local import and remote deletion. Failed collector cleanup does not block subsequent archives; its outstanding state must survive independently of the successful import. A batch paused for an unavailable destination requires explicit resume to avoid unexpected writes when storage returns.

Ordinary archive retrieval remains non-consuming under [ADR 0004](0004-collector-owns-panda-data.md); library delivery additionally requires successful import before removal.
