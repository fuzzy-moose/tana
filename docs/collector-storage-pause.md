# Collector storage pause rationale

Archive admission uses the full size from the HTTP response before writing its body so a known large archive waits for enough storage instead of consuming space and then being discarded. Live checks remain necessary because other writers can consume the reserve during a transfer, and a server can omit its content length. An unknown-length archive is allowed to proceed with those live checks.

Restarting an interrupted archive from zero is an accepted cost; storage pressure is temporary resource unavailability and must not exhaust a job's failure retries. Recovery reserves the full archive size in addition to the normal recovery reserve because that whole archive must be downloaded again. When its full size is unknown, the discarded partial's size is the fallback: counting that cleanup toward recovery could repeatedly restart the same transfer without any external space being freed. The recovery requirement survives collector restarts.

The whole download queue waits for recovery: later downloads must not consume space needed by the interrupted job. Metadata collection remains independent, and delivery of completed archives remains available because successful delivery can free collector storage.

Free space belongs to the filesystem holding collector archives, which can differ from the database and library filesystems. An unsuccessful space check leaves capacity unknown and therefore suspends downloads. Periodic checks and a reserve reduce disk-exhaustion risk but cannot guarantee protection against rapid writes by other processes.
