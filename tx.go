package drel

import (
	"context"
	"time"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/driver"
)

// Tx represents an active database transaction with change tracking.
type Tx struct {
	engine     *Engine
	dbTx       driver.Tx
	tracker    *changeTracker
	heldEvents []any
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
	return n, err
}

func (tx *Tx) queryInternal(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	start := time.Now()
	rows, err := tx.dbTx.Query(ctx, sql, args...)
	tx.engine.notifyQueryHooks(ctx, sql, args, time.Since(start), err)
	return rows, err
}

func (tx *Tx) queryRowInternal(ctx context.Context, sql string, args ...any) Row {
	start := time.Now()
	row := tx.dbTx.QueryRow(ctx, sql, args...)
	tx.engine.notifyQueryHooks(ctx, sql, args, time.Since(start), nil)
	return row
}

// HardRemove marks a tracked entity for permanent (hard) deletion on the next
// flush, bypassing soft delete even when the model supports it.
func (tx *Tx) HardRemove(entity any) error {
	return tx.tracker.MarkHardDeleted(entity)
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

// Add marks an entity for insertion on the next flush.
func (r *TxRepository[T]) Add(entity *T) {
	r.tx.tracker.MarkAdded(entity, r.base)
}

// Remove marks a tracked entity for deletion on the next flush.
func (r *TxRepository[T]) Remove(entity *T) error {
	return r.tx.tracker.MarkDeleted(entity)
}

// Find looks up a single record by primary key and begins tracking it.
func (r *TxRepository[T]) Find(ctx context.Context, id any) (*T, error) {
	qb := newTxQueryBuilder(r.tx, &r.meta)
	result, err := qb.Where(newComparison(r.meta.PKColumn, ast.OpEq, id)).First(ctx)
	if err != nil {
		return nil, err
	}
	if r.meta.Snapshot != nil {
		snap := r.meta.Snapshot(result)
		r.tx.tracker.Track(result, snap, r.base)
	}
	return result, nil
}

// Where starts a filtered query within the transaction.
func (r *TxRepository[T]) Where(pred Predicate) *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta).Where(pred)
}

// All returns all records for this model within the transaction.
func (r *TxRepository[T]) All(ctx context.Context) ([]*T, error) {
	return newTxQueryBuilder(r.tx, &r.meta).All(ctx)
}

// Unscoped returns a query builder with all global filters removed.
func (r *TxRepository[T]) Unscoped() *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta).Unscoped()
}

// WithoutFilter returns a query builder with the named filter removed.
func (r *TxRepository[T]) WithoutFilter(name string) *TxQueryBuilder[T] {
	return newTxQueryBuilder(r.tx, &r.meta).WithoutFilter(name)
}

// TxQueryBuilder constructs and executes typed queries within a transaction.
type TxQueryBuilder[T any] struct {
	tx      *Tx
	meta    *ModelMeta[T]
	wheres  []ast.WhereClause
	orderBy []ast.OrderByExpr
	limit   *int
	offset  *int
	after   *string
	filters []NamedFilter
}

func newTxQueryBuilder[T any](tx *Tx, meta *ModelMeta[T]) *TxQueryBuilder[T] {
	return &TxQueryBuilder[T]{
		tx:      tx,
		meta:    meta,
		filters: append([]NamedFilter(nil), meta.Filters...),
	}
}

func (q *TxQueryBuilder[T]) clone() *TxQueryBuilder[T] {
	c := &TxQueryBuilder[T]{
		tx:      q.tx,
		meta:    q.meta,
		wheres:  make([]ast.WhereClause, len(q.wheres)),
		orderBy: make([]ast.OrderByExpr, len(q.orderBy)),
		limit:   q.limit,
		offset:  q.offset,
		after:   q.after,
		filters: append([]NamedFilter(nil), q.filters...),
	}
	copy(c.wheres, q.wheres)
	copy(c.orderBy, q.orderBy)
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
	pageSize := *q.limit
	order := cursorOrder(q.orderBy, q.meta.PKColumn)

	c := q.clone()
	c.orderBy = order
	c.after = nil
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
	return finishCursorPage(q.meta, order, items, pageSize)
}
