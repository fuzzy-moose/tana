# Go database access

Researched 2026-09-05. Recommendation, not an adopted architecture decision.

The repository currently has independently deployed local and collector services, with HTTP scaffolding and no persistence dependencies. Keep schemas, migrations, and query packages owned by each service. This recommendation assumes local library/reading-progress storage and does not assume a collector deployment topology or measured workload.

## Recommendation

| Service | Starting choice | Reason |
| --- | --- | --- |
| Local comic library | SQLite + `database/sql` + `modernc.org/sqlite` + sqlc | Device-local storage without a database server; generated Go methods from explicit SQL. |
| Cloud metadata collector | Same stack if one instance with persistent local storage; PostgreSQL + `pgx/v5/pgxpool` + sqlc if multiple instances or substantial concurrent writes are planned | Decide from deployment and write concurrency, not the word “cloud.” |

SQLite recommends embedded storage for device-local, low-writer-concurrency workloads and a client/server database when writers cannot take turns or database files are accessed across machines. These deployment recommendations are an inference from that guidance. [SQLite selection guidance](https://www.sqlite.org/whentouse.html)

sqlc's SQLite tutorial generates Go queries over `database/sql` and explicitly uses `modernc.org/sqlite`. Its PostgreSQL integration generates native pgx code and supports `pgxpool`. [SQLite tutorial](https://docs.sqlc.dev/en/latest/tutorials/getting-started-sqlite.html), [pgx integration](https://docs.sqlc.dev/en/latest/guides/using-go-and-pgx.html)

Prefer explicit SQL plus generated typed methods here: library filtering, metadata joins, and reading-progress updates benefit from visible queries. This is a project judgment. GORM offers associations, hooks, eager loading, and automatic migration; choose it if that object-oriented workflow is a concrete requirement. sqlc adds a generation step and requires maintaining SQL. [GORM features](https://gorm.io/docs/), [sqlc generation](https://docs.sqlc.dev/en/latest/howto/generate.html)

## Driver choice

`modernc.org/sqlite` is a CGo-free port of SQLite. Prefer it for simpler Go-only builds; confirm supported target platforms before pinning a version. Its documentation calls out a requirement to match its `modernc.org/libc` dependency version. [Driver documentation](https://pkg.go.dev/modernc.org/sqlite)

`mattn/go-sqlite3` is a valid alternative when CGo is already acceptable or required SQLite extensions favor it. Building requires `CGO_ENABLED=1` and a C compiler; cross-compilation can require a target C toolchain. No workload benchmarks were run, so no performance winner is claimed. [Driver README](https://github.com/mattn/go-sqlite3)

For PostgreSQL, prefer native pgx and its pool. The project recommends its native interface for PostgreSQL-only applications without dependencies requiring `database/sql`; its adapter remains available if needed. [pgx README](https://github.com/jackc/pgx)

## Implementation notes

- Keep one long-lived `*sql.DB` per configured pool. It already manages connections and is safe for concurrent goroutines. A conservative first local configuration is `SetMaxOpenConns(1)`; this serializes reads too. Add a separate read pool only if measured latency warrants it, retaining a single writer connection. Never acquire the same pool through `db` while holding its only connection in a transaction; use the transaction throughout. [Go connection management](https://go.dev/doc/database/manage-connections)
- Enable WAL at startup and verify the resulting mode. WAL permits readers alongside a writer, but still allows only one writer and requires same-host storage. Short transactions avoid holding locks and obstructing checkpoints. [SQLite WAL](https://www.sqlite.org/wal.html)
- Enable foreign keys and a bounded busy timeout on **every connection**, through driver DSN parameters or a connection hook. A one-time `db.Exec` does not configure future pooled connections. Modernc supports repeated `_pragma` parameters, e.g. `file:tana.db?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)`. The five-second timeout is a starting policy, not a guarantee that lock errors disappear. [Foreign keys](https://sqlite.org/foreignkeys.html), [Busy timeout](https://sqlite.org/pragma.html#pragma_busy_timeout), [Modernc DSN options](https://pkg.go.dev/modernc.org/sqlite#Driver.Open)
- Keep versioned SQL migrations beside each service's queries; run migrations through a migration tool or explicit runner. sqlc generates query code and can parse migrations as schema input; it does not apply migrations. [sqlc schema changes](https://docs.sqlc.dev/en/latest/howto/ddl.html)
- Verify constraints, transactions, migrations, and representative queries against the real chosen engine. Generated types do not prove runtime behavior or deployment compatibility. sqlc's enhanced database-backed analyzer currently supports PostgreSQL; SQLite support is planned. [sqlc analysis limits](https://docs.sqlc.dev/en/latest/howto/generate.html)

Do not promise transparent SQLite/PostgreSQL switching: each sqlc configuration selects an engine and generated driver API. Keep service-owned SQL and migrations, and expose domain operations where callers need a persistence boundary. [sqlc configuration](https://docs.sqlc.dev/en/latest/reference/config.html)
