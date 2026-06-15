package drel

import (
	"context"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/driver"
)

// Batch groups multiple read queries so they execute in a single network
// round-trip when the driver supports pipelining (pgx), falling back to
// sequential execution otherwise. Queue queries with BatchAll/BatchFirst/
// BatchCount, call Execute, then read each typed result via Result.
//
//	batch := db.NewBatch()
//	users := drel.BatchAll(batch, db.Users.Where(Users.Active.IsTrue()))
//	count := drel.BatchCount(batch, db.Users)
//	if err := batch.Execute(ctx); err != nil { ... }
//	us, _ := users.Result()
//	n, _ := count.Result()
type Batch struct {
	engine *Engine
	items  []batchItem
}

type batchItem struct {
	sql     string
	args    []any
	primary bool
	handler func(Rows) error
}

// NewBatch creates an empty query batch bound to this engine.
func (e *Engine) NewBatch() *Batch { return &Batch{engine: e} }

func (b *Batch) add(sql string, args []any, primary bool, handler func(Rows) error) {
	b.items = append(b.items, batchItem{sql: sql, args: args, primary: primary, handler: handler})
}

// Execute runs all queued queries. On a pipelining driver they are sent in one
// round-trip and their results read in order; otherwise they run sequentially.
// A pipeline runs on a single connection, so the whole batch targets one driver:
// the primary if any queued query forced Primary(), otherwise a read replica
// (chosen round-robin, falling back to the primary on failure).
func (b *Batch) Execute(ctx context.Context) error {
	if len(b.items) == 0 {
		return nil
	}
	forcePrimary := false
	for _, it := range b.items {
		if it.primary {
			forcePrimary = true
			break
		}
	}

	target := b.engine.rowDriver(ctx, forcePrimary) // primary, or a non-failed replica
	if p, ok := target.(driver.Pipeliner); ok {
		items := make([]driver.BatchItem, len(b.items))
		for i, it := range b.items {
			items[i] = driver.BatchItem{SQL: it.sql, Args: it.args}
		}
		res, err := p.SendBatch(ctx, items)
		if err != nil {
			// Fall back to the primary if the replica pipeline failed to send.
			if !forcePrimary && target != b.engine.drv {
				return b.executeSequential(ctx, b.engine.drv)
			}
			return dberr.Classify(err)
		}
		defer res.Close()
		for _, it := range b.items {
			rows, err := res.Query()
			if err != nil {
				return dberr.Classify(err)
			}
			if err := it.handler(rows); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
		}
		return nil
	}

	// Sequential fallback (e.g. SQLite, or a replica without pipelining).
	return b.executeSequential(ctx, target)
}

func (b *Batch) executeSequential(ctx context.Context, d driver.Driver) error {
	for _, it := range b.items {
		rows, err := b.engine.queryOn(ctx, d, it.sql, it.args...)
		if err != nil {
			return err // already classified by queryOn
		}
		if err := it.handler(rows); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	return nil
}

// BatchResult holds the typed outcome of a batched query, available after the
// batch has been executed.
type BatchResult[T any] struct {
	value T
	err   error
}

// Result returns the query's result and any error encountered while scanning it.
func (r *BatchResult[T]) Result() (T, error) { return r.value, r.err }

// BatchAll queues a query returning all matching rows.
func BatchAll[T any](b *Batch, q *QueryBuilder[T]) *BatchResult[[]*T] {
	res := &BatchResult[[]*T]{}
	built := q.engine.dialect().BuildSelect(q.buildAST(ast.QuerySelect))
	b.add(built.SQL, built.Args, q.primary, func(rows Rows) error {
		var items []*T
		for rows.Next() {
			it, err := q.meta.Scan(rows)
			if err != nil {
				res.err = err
				return err
			}
			items = append(items, it)
		}
		if err := rows.Err(); err != nil {
			res.err = err
			return err
		}
		res.value = items
		return nil
	})
	return res
}

// BatchFirst queues a query returning the first matching row; Result returns
// ErrNotFound if there is none.
func BatchFirst[T any](b *Batch, q *QueryBuilder[T]) *BatchResult[*T] {
	res := &BatchResult[*T]{}
	built := q.engine.dialect().BuildSelect(q.Limit(1).buildAST(ast.QuerySelect))
	b.add(built.SQL, built.Args, q.primary, func(rows Rows) error {
		if rows.Next() {
			it, err := q.meta.Scan(rows)
			if err != nil {
				res.err = err
				return err
			}
			res.value = it
			return rows.Err()
		}
		if err := rows.Err(); err != nil {
			res.err = err
			return err
		}
		res.err = ErrNotFound
		return nil
	})
	return res
}

// BatchCount queues a COUNT query.
func BatchCount[T any](b *Batch, q *QueryBuilder[T]) *BatchResult[int] {
	res := &BatchResult[int]{}
	built := q.engine.dialect().BuildSelect(q.buildAST(ast.QueryCount))
	b.add(built.SQL, built.Args, q.primary, func(rows Rows) error {
		if rows.Next() {
			var n int
			if err := rows.Scan(&n); err != nil {
				res.err = err
				return err
			}
			res.value = n
		}
		return rows.Err()
	})
	return res
}
