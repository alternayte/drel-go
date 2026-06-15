package drel

import (
	"context"
	"fmt"
	"reflect"

	"github.com/alternayte/drel/internal/ast"
)

// ColumnRef identifies a database column for use in Select projections.
type ColumnRef struct {
	name string
}

// ColRef creates a ColumnRef from a column name string.
func ColRef(name string) ColumnRef {
	return ColumnRef{name: name}
}

// Name returns the column name.
func (c ColumnRef) Name() string {
	return c.name
}

// Select executes a projection query, returning only specified columns into DTO type R.
// Because Go does not allow new type parameters on methods, this is a standalone function
// that takes a *QueryBuilder[T] and projects into a different type R.
func Select[R any, T any](ctx context.Context, q *QueryBuilder[T], cols ...ColumnRef) ([]*R, error) {
	plan := getScanPlan(reflect.TypeOf((*R)(nil)).Elem())

	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = c.name
	}

	if err := plan.validateColumns(colNames); err != nil {
		return nil, err
	}

	node := q.buildAST(ast.QuerySelect)
	node.Columns = colNames

	result := q.engine.dialect().BuildSelect(node)
	rows, err := q.engine.queryRouted(ctx, q.primary, result.SQL, result.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*R
	rt := reflect.TypeOf((*R)(nil)).Elem()
	for rows.Next() {
		v := reflect.New(rt)
		dests, err := plan.scanDestFor(v, colNames)
		if err != nil {
			return nil, err
		}
		if err := rows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("drel: select scan: %w", err)
		}
		items = append(items, v.Interface().(*R))
	}
	return items, rows.Err()
}

// AggExpr represents an aggregate function call.
type AggExpr struct {
	fn           ast.AggFunc
	column       string
	distinct     bool
	coalesceZero bool
}

// Sum creates a SUM aggregate expression. Over an empty set it returns 0 via
// COALESCE so it scans cleanly into a non-nullable numeric result type.
func Sum(col ColumnRef) AggExpr { return AggExpr{fn: ast.AggSum, column: col.name, coalesceZero: true} }

// Avg creates an AVG aggregate expression.
func Avg(col ColumnRef) AggExpr { return AggExpr{fn: ast.AggAvg, column: col.name} }

// Min creates a MIN aggregate expression.
func Min(col ColumnRef) AggExpr { return AggExpr{fn: ast.AggMin, column: col.name} }

// Max creates a MAX aggregate expression.
func Max(col ColumnRef) AggExpr { return AggExpr{fn: ast.AggMax, column: col.name} }

// CountCol creates a COUNT aggregate expression for a specific column.
func CountCol(col ColumnRef) AggExpr { return AggExpr{fn: ast.AggCount, column: col.name} }

// CountStar creates a COUNT(*) aggregate expression that counts all rows in the
// group (or the whole result set). Unlike CountCol it counts NULL rows too.
func CountStar() AggExpr { return AggExpr{fn: ast.AggCount, column: ""} }

// Count is an alias for CountStar reading naturally as Count().
func Count() AggExpr { return AggExpr{fn: ast.AggCount, column: ""} }

// CountDistinct creates a COUNT(DISTINCT col) aggregate expression that counts
// the number of distinct non-NULL values in col.
func CountDistinct(col ColumnRef) AggExpr {
	return AggExpr{fn: ast.AggCount, column: col.name, distinct: true}
}

// Aggregate executes a single aggregate function and returns the scalar result.
func Aggregate[R any, T any](ctx context.Context, q *QueryBuilder[T], agg AggExpr) (R, error) {
	var zero R
	node := q.buildAST(ast.QuerySelect)
	node.Columns = nil
	node.Aggregates = []ast.AggregateExpr{
		{Func: agg.fn, Column: agg.column, Alias: "result", Distinct: agg.distinct, CoalesceZero: agg.coalesceZero},
	}

	result := q.engine.dialect().BuildSelect(node)
	row := q.engine.queryRowRouted(ctx, q.primary, result.SQL, result.Args...)
	var val R
	if err := row.Scan(&val); err != nil {
		return zero, fmt.Errorf("drel: aggregate: %w", err)
	}
	return val, nil
}

// GroupSpec specifies a GROUP BY column.
type GroupSpec struct {
	column string
}

// Group creates a GroupSpec from a ColumnRef.
func Group(col ColumnRef) GroupSpec {
	return GroupSpec{column: col.name}
}

// AliasedAgg pairs an aggregate with a result alias.
type AliasedAgg struct {
	alias string
	agg   AggExpr
}

// As pairs an aggregate expression with a column alias for the result.
func As(alias string, agg AggExpr) AliasedAgg {
	return AliasedAgg{alias: alias, agg: agg}
}

// GroupByOpt is an option for GroupBy queries.
type GroupByOpt func(*groupByConfig)

type groupByConfig struct {
	having *Predicate
}

// Having adds a HAVING clause to a GroupBy query.
func Having(pred Predicate) GroupByOpt {
	return func(cfg *groupByConfig) { cfg.having = &pred }
}

// GroupBy executes a GROUP BY query with aggregates, returning results as []*R.
func GroupBy[R any, T any](ctx context.Context, q *QueryBuilder[T], groups []GroupSpec, aggs []AliasedAgg, opts ...GroupByOpt) ([]*R, error) {
	cfg := &groupByConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	plan := getScanPlan(reflect.TypeOf((*R)(nil)).Elem())

	node := q.buildAST(ast.QuerySelect)

	groupCols := make([]string, len(groups))
	for i, g := range groups {
		groupCols[i] = g.column
	}
	node.Columns = groupCols
	node.GroupBy = groupCols

	aliases := make([]string, len(aggs))
	for i, a := range aggs {
		aliases[i] = a.alias
		node.Aggregates = append(node.Aggregates, ast.AggregateExpr{
			Func:         a.agg.fn,
			Column:       a.agg.column,
			Alias:        a.alias,
			Distinct:     a.agg.distinct,
			CoalesceZero: a.agg.coalesceZero,
		})
	}

	// Output column order is the group columns followed by the aggregate
	// aliases — the same emit order BuildSelect produces. Bind scan
	// destinations to this list by name.
	scanCols := make([]string, 0, len(groupCols)+len(aliases))
	scanCols = append(scanCols, groupCols...)
	scanCols = append(scanCols, aliases...)

	if err := plan.validateColumns(scanCols); err != nil {
		return nil, err
	}

	if cfg.having != nil {
		clause := cfg.having.ToAST()
		node.Having = &clause
	}

	result := q.engine.dialect().BuildSelect(node)
	rows, err := q.engine.queryRouted(ctx, q.primary, result.SQL, result.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rt := reflect.TypeOf((*R)(nil)).Elem()
	var items []*R
	for rows.Next() {
		v := reflect.New(rt)
		dests, err := plan.scanDestFor(v, scanCols)
		if err != nil {
			return nil, err
		}
		if err := rows.Scan(dests...); err != nil {
			return nil, fmt.Errorf("drel: groupby scan: %w", err)
		}
		items = append(items, v.Interface().(*R))
	}
	return items, rows.Err()
}
