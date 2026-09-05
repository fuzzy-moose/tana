# Tana

Comic management and reading locally, with a separate cloud metadata collector.

## Structure and responsibilities

One Go module, two independently deployed services, and a React TypeScript SPA.

| Path                  | Responsibility                                                                                       |
|-----------------------|------------------------------------------------------------------------------------------------------|
| `cmd/tana/`           | Local service entry point: configuration, logging, and process lifecycle.                            |
| `cmd/collector/`      | Cloud collector entry point: configuration, logging, and process lifecycle.                          |
| `internal/local/`     | Local application behavior; `httpapi/` owns its HTTP routes.                                         |
| `internal/collector/` | Cloud metadata collection behavior; `httpapi/` owns its HTTP routes.                                 |
| `internal/server/`    | Shared HTTP infrastructure: configuration, request logging, health responses, and graceful shutdown. |
| `web/`                | React UI and Vite tooling; reader interaction and presentation belong here.                          |

The local service owns comic files, library management, and reading progress.
The collector fetches and caches upstream metadata. The local service pulls that
metadata; the browser communicates with the local service.
