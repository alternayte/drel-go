package drel

import (
	"context"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/driver"
)

// Tx represents an active database transaction with change tracking.
type Tx struct {
	engine  *Engine
	dbTx    driver.Tx
	tracker *changeTracker
}

// SaveChanges flushes all tracked changes within this transaction.
func (tx *Tx) SaveChanges(ctx context.Context) error {
	return flushChanges(ctx, tx.dbTx, tx.engine.Dialect(), tx.tracker)
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

// TxQueryBuilder constructs and executes typed queries within a transaction.
type TxQueryBuilder[T any] struct {
	tx      *Tx
	meta    *ModelMeta[T]
	wheres  []ast.WhereClause
	orderBy []ast.OrderByExpr
	limit   *int
	offset  *int
}

func newTxQueryBuilder[T any](tx *Tx, meta *ModelMeta[T]) *TxQueryBuilder[T] {
	return &TxQueryBuilder[T]{tx: tx, meta: meta}
}

func (q *TxQueryBuilder[T]) clone() *TxQueryBuilder[T] {
	c := &TxQueryBuilder[T]{
		tx:      q.tx,
		meta:    q.meta,
		wheres:  make([]ast.WhereClause, len(q.wheres)),
		orderBy: make([]ast.OrderByExpr, len(q.orderBy)),
		limit:   q.limit,
		offset:  q.offset,
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

func (q *TxQueryBuilder[T]) buildAST(queryType ast.QueryType) ast.SelectNode {
	node := ast.SelectNode{
		Table:   q.meta.Table,
		Columns: q.meta.Columns,
		OrderBy: q.orderBy,
		Limit:   q.limit,
		Offset:  q.offset,
		Type:    queryType,
	}
	if len(q.wheres) == 1 {
		node.Where = &q.wheres[0]
	} else if len(q.wheres) > 1 {
		combined := ast.WhereClause{LogicalOp: ast.LogicalAnd, Children: q.wheres}
		node.Where = &combined
	}
	return node
}

// All executes the query and returns all matching results.
func (q *TxQueryBuilder[T]) All(ctx context.Context) ([]*T, error) {
	node := q.buildAST(ast.QuerySelect)
	result := q.tx.engine.Dialect().BuildSelect(node)
	rows, err := q.tx.dbTx.Query(ctx, result.SQL, result.Args...)
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

// Count returns the number of matching rows.
func (q *TxQueryBuilder[T]) Count(ctx context.Context) (int, error) {
	node := q.buildAST(ast.QueryCount)
	result := q.tx.engine.Dialect().BuildSelect(node)
	row := q.tx.dbTx.QueryRow(ctx, result.SQL, result.Args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
