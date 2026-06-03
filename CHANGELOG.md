# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres
to [Semantic Versioning](https://semver.org/). While the major version is `0`,
minor versions may contain breaking changes.

## [0.3.2] - 2026-06-03

Production-hardening.

### Added
- **Typed, dialect-neutral errors.** `errors.Is(err, drel.ErrUniqueViolation)`
  and `ErrForeignKeyViolation` / `ErrNotNullViolation` / `ErrCheckViolation` /
  `ErrSerializationFailure`, classified uniformly across Postgres (SQLSTATE),
  SQLite (result codes), and LibSQL (message match). The original driver error
  (e.g. `*pgconn.PgError`) remains reachable via `errors.As`.
- **Connection pool configuration:** `WithMaxConns`, `WithConnMaxLifetime`,
  `WithConnMaxIdleTime` (applied to Postgres, SQLite, and LibSQL pools).

### Fixed
- **Bulk parameter-limit overflow.** `BulkInsert`/`BulkUpsert` sized batches at a
  fixed 1000 rows, which overflowed the per-statement parameter limit for wide
  tables (e.g. >65 columns on Postgres, fewer on SQLite). Batch size is now
  derived from the column count.

## [0.3.1] - 2026-06-03

### Changed
- **LibSQL/Turso works out of the box.** Removed the `libsql` build tag: a
  `libsql://` / `https://` / `wss://` URL now just works with no build flags or
  extra imports, alongside `postgres://`, `file:`, etc. The libSQL client (all
  pure Go, no CGO) is compiled into every build. Removed `ErrLibSQLNotBuilt`.

## [0.3.0] - 2026-06-03

A large release that takes drel from a Postgres-only core to a multi-dialect ORM
with EF Core-style change tracking, robust migrations, observability, and scale
features. It supersedes the unreleased 0.2.0 development line. Everything below
is new since `v0.1.0`.

### Added

#### Dialects & drivers
- **SQLite** dialect and driver via pure-Go `modernc.org/sqlite` (no CGO),
  auto-detected from `file:`, `sqlite://`, `:memory:`, or `*.db` DSNs. WAL,
  busy-timeout, and foreign-keys pragmas; in-memory DBs are pinned to one
  connection.
- **LibSQL/Turso** driver (opt-in via the `libsql` build tag), reusing the
  SQLite-compatible dialect. Detects `libsql://` / `https://` / `http://` /
  `wss://` / `ws://`; `WithAuthToken` injects the Turso token; clear
  `ErrLibSQLNotBuilt` when used without the tag. Verified end-to-end against a
  real `libsql-server` over HTTP.
- DSN-based dialect auto-detection in `NewEngine`; `WithDriver`/`WithDialect`
  overrides. Non-RETURNING mutation path (insert readback) for SQLite/LibSQL.

#### Querying & change tracking
- **UnitOfWork** (`db.NewUnitOfWork()`): EF Core DbContext-style change tracking
  with typed, tracked repositories (`uow.Users.Add/Find/Remove/Attach/Detach/
  AsNoTracking`) and `uow.SaveChanges`.
- Tracked queries, `AsNoTracking`, `Attach`/`Detach`, and nested `Savepoint`s on
  the transaction API.
- Offset pagination (`PageOffset`) and keyset/cursor pagination (`Page`,
  `After`/`Take`) with a deterministic primary-key tiebreaker.
- Projections and aggregations into DTOs: `Select`, `Aggregate`, `GroupBy`
  (+ `GroupBy`/`Having`/aggregate AST nodes), plus a reflection-based DTO scanner
  used only for ad-hoc DTOs.
- Nested includes (`Include(Users.Posts.Then(Posts.Tags))`) and refinable
  includes (`Where`/`OrderBy`/`Limit`/`Unscoped`/`WithoutFilter` per relation);
  `IncludableQuery` composes with root `Where`/`OrderBy`/pagination.
- `RawQuery[T]` / `RawQueryRow[T]` with per-dialect placeholder rewriting.

#### Migrations & codegen
- Structured **schema-snapshot migration diff**: add/drop tables, add/drop
  columns, type/nullability/default changes, indexes, and enums, persisted via
  `.drel_snapshot.json`. SQLite in-place ALTER limitations and column renames
  are surfaced as loud `-- WARNING`/`-- NOTE` comments rather than silent skips.
- `db:` tag options for indexes and constraints: `unique`, `index`,
  `index=<name>` (composite), `check=<expr>`.
- CLI: `drel init` and `drel seed`; dialect-aware `migrate up/down/status/lint`
  (Postgres and SQLite); generated code is now gofmt-clean.

#### Scale & observability
- Read replicas: `WithReadReplica` round-robins reads; writes/transactions use
  the primary; `Primary()` forces read-your-writes.
- Query batching: `NewBatch` + `BatchAll`/`BatchFirst`/`BatchCount` over the pgx
  pipeline, with a sequential fallback.
- Transactional outbox: `Engine.UseOutbox` writes events to an outbox table
  within the SaveChanges transaction; `OutboxSchema` DDL helper.
- Observability: `WithLogger` (slog), `WithQueryLog`, `WithSlowQueryThreshold`,
  `WithTracer` (OpenTelemetry-adaptable), and `WithDevMode` diagnostics (N+1
  heuristic, unbounded-query and unused-tracking warnings, Postgres EXPLAIN-based
  missing-index hints).

#### Testing
- `dreltest` package: `NewSQLite` (in-memory) and `Begin` (savepoint isolation);
  `dreltest/pgtest.NewPostgres` (testcontainers) with `WithMigrations`/`WithSeed`.

### Changed
- Documentation (PRD, README) reconciled with the implemented API; performance
  claims replaced with measured benchmark numbers (no sqlc comparison claimed).
- Migrations are powered by a built-in differ — drel does **not** depend on Atlas.

### Fixed
- `COUNT` no longer emitted trailing `ORDER BY`/`LIMIT`/`OFFSET` (broke offset
  pagination totals).
- Migration version collisions when two migrations were created in the same
  second; down migrations now undo up in reverse order.
- Cursor pagination now encodes named order-key types (enums, `uuid.UUID`, value
  objects); the over-fetch sentinel row is no longer tracked.
- `Attach(StateUnchanged)` no longer panics on insert-only models; nested
  same-name savepoints no longer collide.

## [0.1.0]

Initial release: Postgres (pgx) core, code generation (model scanning, query
builders, scan/snapshot/diff), basic CRUD, snapshot-based change tracking,
implicit transactions, and the type-safe query builder.

[0.3.2]: https://github.com/alternayte/drel-go/releases/tag/v0.3.2
[0.3.1]: https://github.com/alternayte/drel-go/releases/tag/v0.3.1
[0.3.0]: https://github.com/alternayte/drel-go/releases/tag/v0.3.0
[0.1.0]: https://github.com/alternayte/drel-go/releases/tag/v0.1.0
