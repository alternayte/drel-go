# Drel

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Code-generation-based Go ORM for Postgres. Type-safe queries,
snapshot-based change tracking, zero runtime reflection.

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

// Insert via transaction with change tracking
err = database.Transaction(ctx, func(tx *drel.Tx) error {
    repo := drel.NewTxRepository(tx, models.TaskMeta)
    repo.Add(models.NewTask("Build ORM", 1))
    return nil
})

// Query with generated type-safe columns
tasks, err := database.Tasks.
    Where(models.Tasks.Done.IsFalse()).
    OrderBy(models.Tasks.Priority.Asc()).
    All(ctx)

// Update with change tracking (only modified columns are UPDATEd)
err = database.Transaction(ctx, func(tx *drel.Tx) error {
    repo := drel.NewTxRepository(tx, models.TaskMeta)
    task, err := repo.Find(ctx, 1)
    if err != nil {
        return err
    }
    task.MarkDone()
    return nil
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
- **Domain events** -- register and dispatch events on entity lifecycle.
- **Raw SQL escape hatches** -- `Engine.Exec`, `Engine.Query`,
  `Engine.QueryRow`, and `Tx.Exec`, `Tx.QueryRow` for anything the
  ORM does not cover.

## Examples

See [examples/](examples/) for working samples:

- [getting-started](examples/getting-started/) -- minimal CRUD
- [model-features](examples/model-features/) -- soft delete, versioning, audit
- [relationships](examples/relationships/) -- associations and includes
- [bulk-ops](examples/bulk-ops/) -- batch operations
- [multi-model](examples/multi-model/) -- domain events, transaction hooks

## Limitations

- Postgres only (SQLite/LibSQL planned).
- Migration generation produces full schema; incremental diffing is not
  yet automated — treat generated SQL as a scaffold and review before
  applying.
- Bulk `Set` accepts `any` values — type safety is enforced on column
  predicates and `FindByID` but not on bulk mutation values.

## License

MIT -- see [LICENSE](LICENSE).
