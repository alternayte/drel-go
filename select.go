package drel

import (
	"context"
	"fmt"
	"reflect"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
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

// QualifiedCol is a table-qualified column reference (table.column) used to build
// JOIN ON expressions and cross-table projections.
type QualifiedCol struct {
	table  string
	column string
}

// QualifiedColRef creates a table-qualified column reference.
func QualifiedColRef(table, column string) QualifiedCol {
	return QualifiedCol{table: table, column: column}
}

// Ref returns a ColumnRef whose name is the qualified "table.column" string, for
// use as a Select projection column. The dialect quotes each segment.
func (q QualifiedCol) Ref() ColumnRef {
	return ColumnRef{name: q.table + "." + q.column}
}

// Qualified returns the raw "table.column" string.
func (q QualifiedCol) Qualified() string {
	return q.table + "." + q.column
}

// JoinOn is a fully-qualified ON expression for a JOIN clause. It is produced by
// QualifiedCol.EqCol and consumed by QueryBuilder.LeftJoin/InnerJoin.
type JoinOn struct {
	sql string
}

// EqCol builds an equality ON expression between two qualified columns, with each
// identifier segment quoted (e.g. "categories"."name" = "products"."category").
func (q QualifiedCol) EqCol(other QualifiedCol) JoinOn {
	return JoinOn{sql: quoteQualified(q) + " = " + quoteQualified(other)}
}

func quoteQualified(q QualifiedCol) string {
	return `"` + escapeIdent(q.table) + `"."` + escapeIdent(q.column) + `"`
}

func escapeIdent(s string) string {
	return replaceAllDoubleQuote(s)
}

func replaceAllDoubleQuote(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// projectable is satisfied by both *QueryBuilder[T] and *TxQueryBuilder[T],
// letting Select/Aggregate/GroupBy run against either the engine or an active
// transaction. It is unexported so it is not part of the public API.
type projectable[T any] interface {
	buildAST(ast.QueryType) ast.SelectNode
	metaPtr() *ModelMeta[T]
	projectionDialect() dialect.Dialect
	queryRows(ctx context.Context, sql string, args ...any) (Rows, error)
	queryRow(ctx context.Context, sql string, args ...any) Row
}

// Select executes a projection query, returning only specified columns into DTO type R.
// It accepts either an engine-level *QueryBuilder[T] or a transaction-bound
// *TxQueryBuilder[T]; in the latter case the projection runs inside the transaction.
func Select[R any, T any](ctx context.Context, q projectable[T], cols ...ColumnRef) ([]*R, error) {
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

	result := q.projectionDialect().BuildSelect(node)
	rows, err := q.queryRows(ctx, result.SQL, result.Args...)
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
// Accepts an engine-level *QueryBuilder[T] or a transaction-bound *TxQueryBuilder[T].
func Aggregate[R any, T any](ctx context.Context, q projectable[T], agg AggExpr) (R, error) {
	var zero R
	node := q.buildAST(ast.QuerySelect)
	node.Columns = nil
	node.Aggregates = []ast.AggregateExpr{
		{Func: agg.fn, Column: agg.column, Alias: "result", Distinct: agg.distinct, CoalesceZero: agg.coalesceZero},
	}

	result := q.projectionDialect().BuildSelect(node)
	row := q.queryRow(ctx, result.SQL, result.Args...)
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
// Accepts an engine-level *QueryBuilder[T] or a transaction-bound *TxQueryBuilder[T].
func GroupBy[R any, T any](ctx context.Context, q projectable[T], groups []GroupSpec, aggs []AliasedAgg, opts ...GroupByOpt) ([]*R, error) {
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

	result := q.projectionDialect().BuildSelect(node)
	rows, err := q.queryRows(ctx, result.SQL, result.Args...)
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
