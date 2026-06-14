package drel

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/driver"
)

// Tx represents an active database transaction with change tracking.
type Tx struct {
	engine     *Engine
	dbTx       driver.Tx
	tracker    *changeTracker
	heldEvents []any
	spCounter  int
}

// SaveChanges flushes all tracked changes within this transaction.
func (tx *Tx) SaveChanges(ctx context.Context) error {
	events, err := flushChanges(ctx, tx, tx.engine.dialect(), tx.tracker)
	if err != nil {
		return err
	}
	tx.heldEvents = append(tx.heldEvents, events...)
	return nil
}

// Exec executes a raw SQL statement within the transaction.
func (tx *Tx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return tx.execInternal(ctx, sql, args...)
}

// QueryRow executes a raw SQL query within the transaction that returns at most one row.
func (tx *Tx) QueryRow(ctx context.Context, sql string, args ...any) Row {
	return tx.queryRowInternal(ctx, sql, args...)
}

func (tx *Tx) execInternal(ctx context.Context, sql string, args ...any) (int64, error) {
	start := time.Now()
	n, err := tx.dbTx.Exec(ctx, sql, args...)
	tx.engine.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return n, dberr.Classify(err)
}

func (tx *Tx) queryInternal(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := tx.dbTx.Query(ctx, sql, args...)
	tx.engine.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return rows, dberr.Classify(err)
}

func (tx *Tx) queryRowInternal(ctx context.Context, sql string, args ...any) Row {
	start := time.Now()
	row := tx.dbTx.QueryRow(ctx, sql, args...)
	tx.engine.notifyQueryHooks(ctx, sql, args, time.Since(start), nil)
	return classifyRow{row: row}
}

// HardRemove marks a tracked entity for permanent (hard) deletion on the next
// flush, bypassing soft delete even when the model supports it.
func (tx *Tx) HardRemove(entity any) error {
	return tx.tracker.MarkHardDeleted(entity)
}

// Savepoint runs fn within a nested SAVEPOINT. If fn returns nil the savepoint
// is released; otherwise the transaction is rolled back to the savepoint and
// the change tracker is reverted to its state before the savepoint, so entities
// added inside fn are dropped and prior entities keep their earlier state. The
// outer transaction continues either way. Savepoints may be nested, including
// reusing the same name.
//
// Note: rollback reverts the change tracker, not the in-memory Go structs. If a
// flush inside fn populated generated fields on an entity (id, version,
// created_at) and the savepoint is then rolled back, that entity still carries
// those values in memory even though its row no longer exists. Discard such
// entities rather than reusing them after a rolled-back savepoint.
func (tx *Tx) Savepoint(ctx context.Context, name string, fn func(sp *Tx) error) error {
	// A per-transaction counter guarantees a unique SQL identifier even when the
	// same user-facing name is reused at different nesting levels.
	tx.spCounter++
	sp := fmt.Sprintf("%s_%d", sanitizeSavepoint(name), tx.spCounter)
	savedTracker := tx.tracker.save()
	savedEvents := len(tx.heldEvents)

	if _, err := tx.execInternal(ctx, "SAVEPOINT "+sp); err != nil {
		return fmt.Errorf("drel: savepoint %q: %w", name, err)
	}

	if err := fn(tx); err != nil {
		if _, rbErr := tx.execInternal(ctx, "ROLLBACK TO SAVEPOINT "+sp); rbErr != nil {
			return fmt.Errorf("drel: rollback to savepoint %q: %w (while handling: %v)", name, rbErr, err)
		}
		// Release the (now rewound) savepoint so its name is reusable.
		_, _ = tx.execInternal(ctx, "RELEASE SAVEPOINT "+sp)
		tx.tracker.restore(savedTracker)
		tx.heldEvents = tx.heldEvents[:savedEvents]
		return err
	}

	if _, err := tx.execInternal(ctx, "RELEASE SAVEPOINT "+sp); err != nil {
		return fmt.Errorf("drel: release savepoint %q: %w", name, err)
	}
	return nil
}

// sanitizeSavepoint produces a safe SQL identifier from a user-supplied name.
// Savepoint names cannot be parameterized, so disallowed characters are mapped
// to underscores and a leading letter is guaranteed.
func sanitizeSavepoint(name string) string {
	var b strings.Builder
	b.WriteString("sp_")
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// TxRepository provides tracked query and mutation access within a transaction.
type TxRepository[T any] struct {
	tx   *Tx
	meta ModelMeta[T]
	base *ModelMetaBase
}

// NewTxRepository creates a new TxRepository for the given transaction and model metadata.
func NewTxRepository[T any](tx *Tx, meta ModelMeta[T]) *TxRepository[T] {
	return &TxRepository[T]{
		tx:   tx,
		meta: meta,
		base: ToMetaBase(&meta),
	}
}

// Repo returns a transaction-bound repository for the given model metadata.
// It is sugar for NewTxRepository(tx, meta). (A method form is impossible in Go
// because methods cannot have their own type parameters.)
func Repo[T any](tx *Tx, meta ModelMeta[T]) *TxRepository[T] {
	return NewTxRepository(tx, meta)
}

// Add marks an entity for insertion on the next flush.
func (r *TxRepository[T]) Add(entity *T) {
	r.tx.tracker.MarkAdded(entity, r.base)
}

// Remove marks a tracked entity for deletion on the next flush.
func (r *TxRepository[T]) Remove(entity *T) error {
	return r.tx.tracker.MarkDeleted(entity)
}

// Attach begins tracking an externally-constructed entity (e.g. deserialized
// from a request) in the given state. StateModified flushes a full-column
// UPDATE; StateAdded behaves like Add; StateUnchanged snapshots the entity so
// subsequent mutations are detected.
func (r *TxRepository[T]) Attach(entity *T, state EntityState) {
	r.tx.tracker.Attach(entity, state, r.base)
}

// Detach stops tracking an entity so its mutations are no longer flushed.
func (r *TxRepository[T]) Detach(entity *T) {
	r.tx.tracker.Detach(entity)
}

// AsNoTracking returns a query builder whose results are not tracked, for
// read-only queries within the transaction.
func (r *TxRepository[T]) AsNoTracking() *TxQueryBuilder[T] {
	qb := newTxQueryBuilder(r.tx, &r.meta, r.base)
	qb.noTrack = true
	return qb
}

// Find looks up a single record by primary key and begins tracking it.
func (r *TxRepository[T]) Find(ctx context.Context, id any) (*T, error) {
	qb := newTxQueryBuilder(r.tx, &r.meta, r.base)
	return qb.Where(newComparison(r.meta.PKColumn, ast.OpEq, id)).First(ctx)
}

// Where starts a filtered, tracked query within the transaction.
func (r *TxRepository[T]) Where(pred Predicate) *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta, r.base).Where(pred)
}

// OrderBy starts an ordered, tracked query within the transaction.
func (r *TxRepository[T]) OrderBy(exprs ...OrderExpr) *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta, r.base).OrderBy(exprs...)
}

// All returns all records for this model within the transaction, tracking them.
func (r *TxRepository[T]) All(ctx context.Context) ([]*T, error) {
	return newTxQueryBuilder(r.tx, &r.meta, r.base).All(ctx)
}

// Unscoped returns a query builder with all global filters removed.
func (r *TxRepository[T]) Unscoped() *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta, r.base).Unscoped()
}

// WithoutFilter returns a query builder with the named filter removed.
func (r *TxRepository[T]) WithoutFilter(name string) *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta, r.base).WithoutFilter(name)
}

// TxQueryBuilder constructs and executes typed queries within a transaction.
// By default, results are tracked by the transaction's change tracker so that
// mutations are detected on SaveChanges; use AsNoTracking to opt out.
type TxQueryBuilder[T any] struct {
	tx      *Tx
	meta    *ModelMeta[T]
	base    *ModelMetaBase
	wheres  []ast.WhereClause
	orderBy []ast.OrderByExpr
	limit   *int
	offset  *int
	after   *string
	filters []NamedFilter
	noTrack bool
}

func newTxQueryBuilder[T any](tx *Tx, meta *ModelMeta[T], base *ModelMetaBase) *TxQueryBuilder[T] {
	return &TxQueryBuilder[T]{
		tx:      tx,
		meta:    meta,
		base:    base,
		filters: append([]NamedFilter(nil), meta.Filters...),
	}
}

func (q *TxQueryBuilder[T]) clone() *TxQueryBuilder[T] {
	c := &TxQueryBuilder[T]{
		tx:      q.tx,
		meta:    q.meta,
		base:    q.base,
		wheres:  make([]ast.WhereClause, len(q.wheres)),
		orderBy: make([]ast.OrderByExpr, len(q.orderBy)),
		limit:   q.limit,
		offset:  q.offset,
		after:   q.after,
		filters: append([]NamedFilter(nil), q.filters...),
		noTrack: q.noTrack,
	}
	copy(c.wheres, q.wheres)
	copy(c.orderBy, q.orderBy)
	return c
}

// AsNoTracking returns a copy of the builder whose results will not be tracked.
func (q *TxQueryBuilder[T]) AsNoTracking() *TxQueryBuilder[T] {
	c := q.clone()
	c.noTrack = true
	return c
}

// Where adds a filter predicate. Multiple calls are ANDed together.
func (q *TxQueryBuilder[T]) Where(pred Predicate) *TxQueryBuilder[T] {
	c := q.clone()
	c.wheres = append(c.wheres, pred.clause)
	return c
}

// OrderBy sets the ordering for the query.
func (q *TxQueryBuilder[T]) OrderBy(exprs ...OrderExpr) *TxQueryBuilder[T] {
	c := q.clone()
	for _, e := range exprs {
		c.orderBy = append(c.orderBy, e.ToAST())
	}
	return c
}

// Limit restricts the number of results returned.
func (q *TxQueryBuilder[T]) Limit(n int) *TxQueryBuilder[T] {
	c := q.clone()
	c.limit = &n
	return c
}

// Skip sets the offset for the query (number of rows to skip).
func (q *TxQueryBuilder[T]) Skip(n int) *TxQueryBuilder[T] {
	c := q.clone()
	c.offset = &n
	return c
}

func (q *TxQueryBuilder[T]) buildAST(queryType ast.QueryType) ast.SelectNode {
	node := ast.SelectNode{
		Table:   q.meta.Table,
		Columns: q.meta.Columns,
		OrderBy: q.orderBy,
		Limit:   q.limit,
		Offset:  q.offset,
		Type:    queryType,
	}

	allWheres := make([]ast.WhereClause, 0, len(q.filters)+len(q.wheres))
	for _, f := range q.filters {
		allWheres = append(allWheres, f.Clause)
	}
	allWheres = append(allWheres, q.wheres...)

	if len(allWheres) == 1 {
		node.Where = &allWheres[0]
	} else if len(allWheres) > 1 {
		combined := ast.WhereClause{LogicalOp: ast.LogicalAnd, Children: allWheres}
		node.Where = &combined
	}
	return node
}

// Unscoped returns a new builder with all global filters removed.
func (q *TxQueryBuilder[T]) Unscoped() *TxQueryBuilder[T] {
	c := q.clone()
	c.filters = nil
	return c
}

// WithoutFilter returns a new builder with the named filter removed.
func (q *TxQueryBuilder[T]) WithoutFilter(name string) *TxQueryBuilder[T] {
	c := q.clone()
	var remaining []NamedFilter
	for _, f := range c.filters {
		if f.Name != name {
			remaining = append(remaining, f)
		}
	}
	c.filters = remaining
	return c
}

// All executes the query and returns all matching results.
func (q *TxQueryBuilder[T]) All(ctx context.Context) ([]*T, error) {
	node := q.buildAST(ast.QuerySelect)
	result := q.tx.engine.dialect().BuildSelect(node)
	rows, err := q.tx.queryInternal(ctx, result.SQL, result.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*T
	for rows.Next() {
		item, err := q.meta.Scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !q.noTrack && q.base != nil && q.meta.Snapshot != nil {
		for _, item := range items {
			q.tx.tracker.Track(item, q.meta.Snapshot(item), q.base)
		}
	}
	return items, nil
}

// First returns the first matching result or ErrNotFound if none exist.
func (q *TxQueryBuilder[T]) First(ctx context.Context) (*T, error) {
	limited := q.Limit(1)
	items, err := limited.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items[0], nil
}

// FirstOrNil returns the first matching result or nil if none exist.
func (q *TxQueryBuilder[T]) FirstOrNil(ctx context.Context) (*T, error) {
	limited := q.Limit(1)
	items, err := limited.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return items[0], nil
}

// Count returns the number of matching rows.
func (q *TxQueryBuilder[T]) Count(ctx context.Context) (int, error) {
	node := q.buildAST(ast.QueryCount)
	result := q.tx.engine.dialect().BuildSelect(node)
	row := q.tx.queryRowInternal(ctx, result.SQL, result.Args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Exists returns true if at least one matching row exists.
func (q *TxQueryBuilder[T]) Exists(ctx context.Context) (bool, error) {
	node := q.buildAST(ast.QueryExists)
	result := q.tx.engine.dialect().BuildSelect(node)
	row := q.tx.queryRowInternal(ctx, result.SQL, result.Args...)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// Take limits the number of results returned. Alias for Limit.
func (q *TxQueryBuilder[T]) Take(n int) *TxQueryBuilder[T] {
	return q.Limit(n)
}

// After positions a cursor-paginated query past the row encoded by the cursor.
func (q *TxQueryBuilder[T]) After(cursor string) *TxQueryBuilder[T] {
	c := q.clone()
	c.after = &cursor
	return c
}

// PageOffset executes an offset-based page query within the transaction.
func (q *TxQueryBuilder[T]) PageOffset(ctx context.Context) (*OffsetPage[T], error) {
	if q.limit == nil {
		return nil, ErrPaginationNeedsLimit
	}
	if *q.limit <= 0 {
		return nil, ErrInvalidPageSize
	}
	total, err := q.Count(ctx)
	if err != nil {
		return nil, err
	}
	items, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	offset := 0
	if q.offset != nil {
		offset = *q.offset
	}
	return buildOffsetPage(items, total, *q.limit, offset), nil
}

// Page executes a keyset (cursor) page query within the transaction.
func (q *TxQueryBuilder[T]) Page(ctx context.Context) (*CursorPage[T], error) {
	if len(q.orderBy) == 0 {
		return nil, ErrCursorPaginationNeedsOrderBy
	}
	if q.limit == nil {
		return nil, ErrPaginationNeedsLimit
	}
	if *q.limit <= 0 {
		return nil, ErrInvalidPageSize
	}
	pageSize := *q.limit
	order := cursorOrder(q.orderBy, q.meta.PKColumn)

	c := q.clone()
	c.orderBy = order
	c.after = nil
	c.offset = nil // keyset pagination is mutually exclusive with OFFSET; ignore Skip.
	// Fetch the over-fetch (pageSize+1) without tracking so the discarded
	// sentinel row is never added to the change tracker.
	c.noTrack = true
	if q.after != nil {
		payload, err := decodeCursor(*q.after)
		if err != nil {
			return nil, err
		}
		if err := validateCursorColumns(payload, order); err != nil {
			return nil, err
		}
		c.wheres = append(c.wheres, keysetClause(order, payload.Vals))
	}
	fetch := pageSize + 1
	c.limit = &fetch

	items, err := c.All(ctx)
	if err != nil {
		return nil, err
	}
	page, err := finishCursorPage(q.meta, order, items, pageSize)
	if err != nil {
		return nil, err
	}
	// Track only the rows actually returned to the caller (unless opted out).
	if !q.noTrack && q.base != nil && q.meta.Snapshot != nil {
		for _, item := range page.Items {
			q.tx.tracker.Track(item, q.meta.Snapshot(item), q.base)
		}
	}
	return page, nil
}
