# Drel

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Code-generation-based Go ORM for Postgres and SQLite/LibSQL (Turso).
Type-safe queries, snapshot-based change tracking, zero runtime reflection,
EF Core-level developer experience.

## Installation

```bash
go install github.com/alternayte/drel/cmd/drel@latest
```

## Quick Start

### 1. Define a model

```go
package models

import "github.com/alternayte/drel"

type Task struct {
    drel.Model[int]
    Title    string `db:"title"`
    Done     bool   `db:"done"`
    Priority int    `db:"priority"`
}

func NewTask(title string, priority int) *Task {
    return &Task{Title: title, Priority: priority}
}

func (t *Task) MarkDone() { t.Done = true }
```

### 2. Generate code

```bash
drel generate
```

This produces type-safe query builders, scan functions, snapshot/diff
helpers, and a `DB` struct that aggregates all discovered models.

### 3. Use it

```go
// Open the generated DB
database, err := db.Open(dsn)

// Insert via a UnitOfWork (change-tracking work session)
uow := database.NewUnitOfWork()
uow.Tasks.Add(models.NewTask("Build ORM", 1))
err = uow.SaveChanges(ctx)

// Query with generated type-safe columns (read-only, untracked)
tasks, err := database.Tasks.
    Where(models.Tasks.Done.IsFalse()).
    OrderBy(models.Tasks.Priority.Asc()).
    All(ctx)

// Update with change tracking (only modified columns are UPDATEd)
uow = database.NewUnitOfWork()
task, err := uow.Tasks.Find(ctx, 1) // tracked
if err == nil {
    task.MarkDone()
    err = uow.SaveChanges(ctx)
}

// Or an explicit multi-statement transaction:
err = database.Transaction(ctx, func(tx *drel.Tx) error {
    repo := drel.NewTxRepository(tx, models.TaskMeta)
    repo.Add(models.NewTask("ship it", 2))
    return tx.SaveChanges(ctx)
})
```

## Features

- **No reflection** -- all scanning, diffing, and query building use
  generated code.
- **Snapshot-based change tracking** -- only modified columns appear in
  UPDATE statements.
- **Type-safe query builder** -- compile-time checked column predicates
  and ordering (`Eq`, `In`, `Between`, `ILike`, `Raw`).
- **Transactions** -- explicit transaction API with configurable isolation
  levels and automatic flush on commit.
- **Soft delete, versioning, audit** -- embed `drel.SoftDelete`,
  `drel.Versioned`, or `drel.Audit` for automatic column management.
- **Relationships** -- generated `RelationInfo` and `IncludeSpec` for
  has-many, has-one, belongs-to, and many-to-many eager loading with
  cross-package support. Filter-aware includes respect soft-delete on
  related models, with `Unscoped()` opt-out.
- **Bulk operations** -- `BulkInsert`, `BulkUpdate`, `BulkDelete`,
  `BulkUpsert` with batching and safety guards.
- **Domain events & outbox** -- record events on entities, dispatch them
  after commit, and optionally persist them to a transactional outbox table
  via `Engine.UseOutbox`.
- **Pagination** -- offset (`PageOffset`) and keyset/cursor (`Page`) paging
  with a deterministic primary-key tiebreaker.
- **Projections & aggregations** -- `Select`, `Aggregate`, `GroupBy` into
  arbitrary DTOs.
- **Nested & filtered includes** -- `Include(Users.Posts.Then(Posts.Tags))`,
  with `Where`/`OrderBy`/`Limit` per relationship; split-query loading avoids
  cartesian products.
- **Change-tracking depth** -- tracked queries by default, `AsNoTracking`,
  `Attach`/`Detach`, and nested `Savepoint`s.
- **Migrations** -- dialect-aware schema generation and a structured snapshot
  diff (`drel migrate new`) that emits add/drop/alter for tables, columns,
  types, nullability, and indexes; `up`/`down`/`status`/`lint` for both
  dialects. Declare indexes/checks with `db:` tag options.
- **Read replicas** -- `WithReadReplica` round-robins reads; writes and
  transactions use the primary; `Primary()` forces read-your-writes.
- **Query batching** -- `NewBatch` + `BatchAll`/`BatchFirst`/`BatchCount`
  pipeline queries over pgx (sequential fallback elsewhere).
- **Observability** -- structured `slog` query logging, slow-query and
  dev-mode diagnostics (N+1, unbounded queries, missing-index hints), and an
  OpenTelemetry-adaptable `Tracer`.
- **CLI** -- `drel init`, `generate`, `migrate`, `seed`.
- **Raw SQL escape hatches** -- `Engine.Exec`, `Engine.Query`,
  `Engine.QueryRow`, `RawQuery[T]`, and `Tx.Exec`, `Tx.QueryRow` for anything
  the ORM does not cover.

## Examples

See [examples/](examples/) for working samples:

- [getting-started](examples/getting-started/) -- minimal CRUD
- [sqlite-todo](examples/sqlite-todo/) -- SQLite dialect, tag indexes, cursor pagination
- [model-features](examples/model-features/) -- soft delete, versioning, audit
- [relationships](examples/relationships/) -- associations and includes
- [bulk-ops](examples/bulk-ops/) -- batch operations
- [multi-model](examples/multi-model/) -- domain events, transaction hooks

## Dialects

- **Postgres** — direct `pgx`, auto-detected from `postgres://` DSNs.
- **SQLite** — pure-Go `modernc.org/sqlite`, auto-detected from `file:`,
  `sqlite://`, `:memory:`, or `*.db` DSNs.
- **LibSQL/Turso** — `libsql://`/`https://`/`wss://` DSNs, no build flags or
  imports required. Verified end-to-end against a real libSQL server over HTTP.
  Prefer `libsql://`/`https://` over `ws://` for models with `time.Time` columns.

## Limitations

- Migration diffing does not auto-detect column **renames** — a rename appears
  as drop + add; edit the generated SQL if you intend a rename. SQLite cannot
  `ALTER COLUMN TYPE`/nullability in place, so those changes are emitted as
  loud `-- WARNING` comments to be applied by hand.
- Bulk `Set` accepts `any` values — type safety is enforced on column
  predicates and `Find` but not on bulk mutation values.
- True JOIN-based eager loading is intentionally not offered; relationships
  load via batched split queries (correct for every shape, no cartesian
  products).

## License

MIT -- see [LICENSE](LICENSE).
