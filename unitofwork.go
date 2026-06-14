package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/ast"
)

// UnitOfWork is a change-tracking work session in the Entity Framework
// DbContext style. Entities loaded through its repositories are tracked;
// Add/Remove/Attach stage inserts, deletes, and updates; SaveChanges flushes all
// staged changes in a single transaction and dispatches domain events on commit.
//
// Unlike an explicit transaction (Engine.Transaction), a UnitOfWork does not
// hold a database connection open between calls — reads run on demand and
// SaveChanges opens a short transaction to flush. It is therefore not safe for
// concurrent use; use one per logical operation (e.g. per request).
//
// Typed access (uow.Users.Find/Add/...) is provided by the generated
// UnitOfWork wrapper in your db package; this type carries the shared tracker
// and SaveChanges.
type UnitOfWork struct {
	engine     *Engine
	tracker    *changeTracker
	heldEvents []any
}

// NewUnitOfWork creates a change-tracking work session bound to this engine.
func (e *Engine) NewUnitOfWork() *UnitOfWork {
	return &UnitOfWork{engine: e, tracker: newChangeTracker()}
}

// SaveChanges flushes all staged changes within a single transaction and, on
// success, dispatches recorded domain events to after-commit hooks. The unit of
// work may continue to be used after a successful SaveChanges.
func (u *UnitOfWork) SaveChanges(ctx context.Context) (retErr error) {
	dbTx, err := u.engine.drv.Begin(ctx)
	if err != nil {
		return fmt.Errorf("drel: begin: %w", err)
	}
	tx := &Tx{engine: u.engine, dbTx: dbTx, tracker: u.tracker, heldEvents: u.heldEvents}

	committed := false
	defer func() {
		if p := recover(); p != nil {
			_ = dbTx.Rollback(ctx)
			panic(p)
		}
		if !committed {
			_ = dbTx.Rollback(ctx)
		}
	}()

	events, err := flushChanges(ctx, tx, u.engine.dialect(), u.tracker)
	if err != nil {
		u.tracker.resetFlushed()
		return err
	}
	allEvents := append(tx.heldEvents, events...)

	for _, hook := range u.engine.beforeCommitHooks {
		if err := hook(ctx, tx, allEvents); err != nil {
			u.tracker.resetFlushed()
			return err
		}
	}
	if err := flushHookChanges(ctx, tx, u.engine.dialect(), u.tracker); err != nil {
		u.tracker.resetFlushed()
		return err
	}

	if u.engine.devMode {
		if n := u.tracker.countUnusedTracked(); n > 0 {
			u.engine.devWarn(ctx, "drel dev: tracked entities were loaded but never modified; consider AsNoTracking for read-only queries", "count", n)
		}
	}

	if err := dbTx.Commit(ctx); err != nil {
		u.tracker.resetFlushed()
		return fmt.Errorf("drel: commit: %w", err)
	}
	committed = true
	u.tracker.PostCommit()
	clearPendingEvents(u.tracker)
	u.heldEvents = nil

	for _, hook := range u.engine.afterCommitHooks {
		hook(ctx, allEvents)
	}
	return nil
}

// UoWRepository provides tracked query and mutation access for a model within a
// UnitOfWork. Reads run on the primary connection (read-your-writes) and track
// their results; Add/Remove/Attach/Detach stage changes for the next
// SaveChanges.
type UoWRepository[T any] struct {
	uow  *UnitOfWork
	meta ModelMeta[T]
	base *ModelMetaBase
}

// NewUoWRepository creates a tracked repository bound to a UnitOfWork.
func NewUoWRepository[T any](uow *UnitOfWork, meta ModelMeta[T]) *UoWRepository[T] {
	return &UoWRepository[T]{uow: uow, meta: meta, base: ToMetaBase(&meta)}
}

func (r *UoWRepository[T]) newBuilder() *QueryBuilder[T] {
	qb := newQueryBuilder(r.uow.engine, &r.meta)
	qb.primary = true // tracked reads target the primary for read-your-writes
	qb.tracker = r.uow.tracker
	qb.base = r.base
	return qb
}

// Add stages an entity for insertion on the next SaveChanges.
func (r *UoWRepository[T]) Add(entity *T) { r.uow.tracker.MarkAdded(entity, r.base) }

// Remove stages a tracked entity for deletion on the next SaveChanges.
func (r *UoWRepository[T]) Remove(entity *T) error { return r.uow.tracker.MarkDeleted(entity) }

// HardRemove stages a permanent delete, bypassing soft delete.
func (r *UoWRepository[T]) HardRemove(entity *T) error { return r.uow.tracker.MarkHardDeleted(entity) }

// Attach begins tracking an externally-constructed entity in the given state.
func (r *UoWRepository[T]) Attach(entity *T, state EntityState) {
	r.uow.tracker.Attach(entity, state, r.base)
}

// Detach stops tracking an entity.
func (r *UoWRepository[T]) Detach(entity *T) { r.uow.tracker.Detach(entity) }

// Find looks up a record by primary key and tracks it.
func (r *UoWRepository[T]) Find(ctx context.Context, id any) (*T, error) {
	return r.newBuilder().Where(newComparison(r.meta.PKColumn, ast.OpEq, id)).First(ctx)
}

// Where starts a filtered, tracked query.
func (r *UoWRepository[T]) Where(pred Predicate) *QueryBuilder[T] {
	return r.newBuilder().Where(pred)
}

// OrderBy starts an ordered, tracked query.
func (r *UoWRepository[T]) OrderBy(exprs ...OrderExpr) *QueryBuilder[T] {
	return r.newBuilder().OrderBy(exprs...)
}

// All returns all records, tracking them.
func (r *UoWRepository[T]) All(ctx context.Context) ([]*T, error) {
	return r.newBuilder().All(ctx)
}

// First returns the first record (or ErrNotFound), tracking it.
func (r *UoWRepository[T]) First(ctx context.Context) (*T, error) {
	return r.newBuilder().First(ctx)
}

// Count returns the number of matching records.
func (r *UoWRepository[T]) Count(ctx context.Context) (int, error) {
	return r.newBuilder().Count(ctx)
}

// Exists reports whether any matching record exists.
func (r *UoWRepository[T]) Exists(ctx context.Context) (bool, error) {
	return r.newBuilder().Exists(ctx)
}

// AsNoTracking returns a query builder whose results are not tracked.
func (r *UoWRepository[T]) AsNoTracking() *QueryBuilder[T] {
	qb := newQueryBuilder(r.uow.engine, &r.meta)
	qb.primary = true
	return qb
}

// Unscoped returns a tracked query builder with all global filters removed.
func (r *UoWRepository[T]) Unscoped() *QueryBuilder[T] {
	return r.newBuilder().Unscoped()
}

// Include begins a tracked query that eagerly loads the given relationships.
func (r *UoWRepository[T]) Include(rels ...IncludeSpec) *IncludableQuery[T] {
	repo := &Repository[T]{engine: r.uow.engine, meta: r.meta}
	q := repo.Include(rels...)
	q.builder = r.newBuilder()
	return q
}
