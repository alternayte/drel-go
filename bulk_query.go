package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
)

// BulkUpdate updates all rows matching the builder's WHERE conditions.
// Returns the number of affected rows.
func (q *QueryBuilder[T]) BulkUpdate(ctx context.Context, sets ...SetClause) (int, error) {
	if len(sets) == 0 {
		return 0, fmt.Errorf("drel: bulk update %s: at least one Set clause is required", q.meta.Table)
	}
	cvs := make([]dialect.ColumnValue, len(sets))
	for i, s := range sets {
		cvs[i] = dialect.ColumnValue{Column: s.Column, Value: s.Value}
	}

	where := q.combinedWhere()
	result := q.engine.dialect().BuildBulkUpdate(q.meta.Table, cvs, where)
	affected, err := q.engine.execInternal(ctx, result.SQL, result.Args...)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk update %s: %w", q.meta.Table, err)
	}
	return int(affected), nil
}

// BulkDelete deletes all rows matching the builder's WHERE conditions.
// If the model has soft delete, it performs a soft delete instead.
// Returns the number of affected rows.
func (q *QueryBuilder[T]) BulkDelete(ctx context.Context) (int, error) {
	where := q.combinedWhere()

	if len(q.wheres) == 0 && len(q.filters) == 0 {
		return 0, ErrBulkDeleteRequiresFilter
	}

	if q.meta.HasSoftDelete {
		result := q.engine.dialect().BuildBulkSoftDelete(q.meta.Table, where)
		affected, err := q.engine.execInternal(ctx, result.SQL, result.Args...)
		if err != nil {
			return 0, fmt.Errorf("drel: bulk soft delete %s: %w", q.meta.Table, err)
		}
		return int(affected), nil
	}

	result := q.engine.dialect().BuildBulkDelete(q.meta.Table, where)
	affected, err := q.engine.execInternal(ctx, result.SQL, result.Args...)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk delete %s: %w", q.meta.Table, err)
	}
	return int(affected), nil
}

// combinedWhere merges global filters and user WHERE conditions into a single WhereClause.
func (q *QueryBuilder[T]) combinedWhere() *ast.WhereClause {
	allWheres := make([]ast.WhereClause, 0, len(q.filters)+len(q.wheres))
	for _, f := range q.filters {
		allWheres = append(allWheres, f.Clause)
	}
	allWheres = append(allWheres, q.wheres...)

	if len(allWheres) == 0 {
		return nil
	}
	if len(allWheres) == 1 {
		return &allWheres[0]
	}
	combined := ast.WhereClause{
		LogicalOp: ast.LogicalAnd,
		Children:  allWheres,
	}
	return &combined
}
