package drel

import (
	"context"
	"errors"

	"github.com/alternayte/drel/internal/ast"
)

// ErrNotFound is returned when a query expects a result but finds none.
var ErrNotFound = errors.New("drel: not found")

// QueryBuilder constructs and executes typed queries with an immutable builder pattern.
type QueryBuilder[T any] struct {
	engine  *Engine
	meta    *ModelMeta[T]
	wheres  []ast.WhereClause
	orderBy []ast.OrderByExpr
	limit   *int
	offset  *int
	filters []NamedFilter
}

func newQueryBuilder[T any](engine *Engine, meta *ModelMeta[T]) *QueryBuilder[T] {
	return &QueryBuilder[T]{
		engine:  engine,
		meta:    meta,
		filters: append([]NamedFilter(nil), meta.Filters...),
	}
}

func (q *QueryBuilder[T]) clone() *QueryBuilder[T] {
	c := &QueryBuilder[T]{
		engine:  q.engine,
		meta:    q.meta,
		wheres:  make([]ast.WhereClause, len(q.wheres)),
		orderBy: make([]ast.OrderByExpr, len(q.orderBy)),
		limit:   q.limit,
		offset:  q.offset,
		filters: append([]NamedFilter(nil), q.filters...),
	}
	copy(c.wheres, q.wheres)
	copy(c.orderBy, q.orderBy)
	return c
}

// Where adds a filter predicate. Multiple calls are ANDed together.
func (q *QueryBuilder[T]) Where(pred Predicate) *QueryBuilder[T] {
	c := q.clone()
	c.wheres = append(c.wheres, pred.clause)
	return c
}

// OrderBy sets the ordering for the query.
func (q *QueryBuilder[T]) OrderBy(exprs ...OrderExpr) *QueryBuilder[T] {
	c := q.clone()
	for _, e := range exprs {
		c.orderBy = append(c.orderBy, e.ToAST())
	}
	return c
}

// Limit restricts the number of results returned.
func (q *QueryBuilder[T]) Limit(n int) *QueryBuilder[T] {
	c := q.clone()
	c.limit = &n
	return c
}

// Skip sets the offset for the query (number of rows to skip).
func (q *QueryBuilder[T]) Skip(n int) *QueryBuilder[T] {
	c := q.clone()
	c.offset = &n
	return c
}

func (q *QueryBuilder[T]) buildAST(queryType ast.QueryType) ast.SelectNode {
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
		combined := ast.WhereClause{
			LogicalOp: ast.LogicalAnd,
			Children:  allWheres,
		}
		node.Where = &combined
	}

	return node
}

// Unscoped returns a new builder with all global filters removed.
func (q *QueryBuilder[T]) Unscoped() *QueryBuilder[T] {
	c := q.clone()
	c.filters = nil
	return c
}

// WithoutFilter returns a new builder with the named filter removed.
func (q *QueryBuilder[T]) WithoutFilter(name string) *QueryBuilder[T] {
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
func (q *QueryBuilder[T]) All(ctx context.Context) ([]*T, error) {
	node := q.buildAST(ast.QuerySelect)
	result := q.engine.Dialect().BuildSelect(node)

	rows, err := q.engine.Driver().Query(ctx, result.SQL, result.Args...)
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
func (q *QueryBuilder[T]) First(ctx context.Context) (*T, error) {
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
func (q *QueryBuilder[T]) FirstOrNil(ctx context.Context) (*T, error) {
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
func (q *QueryBuilder[T]) Count(ctx context.Context) (int, error) {
	node := q.buildAST(ast.QueryCount)
	result := q.engine.Dialect().BuildSelect(node)

	row := q.engine.Driver().QueryRow(ctx, result.SQL, result.Args...)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// Exists returns true if at least one matching row exists.
func (q *QueryBuilder[T]) Exists(ctx context.Context) (bool, error) {
	node := q.buildAST(ast.QueryExists)
	result := q.engine.Dialect().BuildSelect(node)

	row := q.engine.Driver().QueryRow(ctx, result.SQL, result.Args...)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}
