package ast

type Operator int

const (
	OpEq Operator = iota
	OpNEQ
	OpGT
	OpGTE
	OpLT
	OpLTE
	OpLike
	OpILike
	OpIn
	OpNotIn
	OpBetween
	OpIsNull
	OpIsNotNull
)

type Direction int

const (
	Asc Direction = iota
	Desc
)

type QueryType int

const (
	QuerySelect QueryType = iota
	QueryCount
	QueryExists
)

type ComparisonNode struct {
	Column string
	Op     Operator
	Value  any
	Values []any
}

type LogicalOp int

const (
	LogicalAnd LogicalOp = iota
	LogicalOr
	LogicalNot
)

type WhereClause struct {
	Comparison *ComparisonNode
	LogicalOp  LogicalOp
	Children   []WhereClause
	Raw        *string
	RawArgs    []any
}

// NullsOrder controls placement of NULL values within an ORDER BY clause.
// NullsDefault omits the clause and lets the database apply its default
// (Postgres: NULLS LAST for ASC, NULLS FIRST for DESC; SQLite: NULLS FIRST
// for ASC, NULLS LAST for DESC).
type NullsOrder int

const (
	NullsDefault NullsOrder = iota
	NullsFirst
	NullsLast
)

type OrderByExpr struct {
	Column    string
	Direction Direction
	Nulls     NullsOrder
}

type AggFunc int

const (
	AggSum AggFunc = iota
	AggAvg
	AggMin
	AggMax
	AggCount
)

type AggregateExpr struct {
	Func   AggFunc
	Column string
	Alias  string
}

// PartitionLimit requests a per-partition row cap rendered with a window
// function (ROW_NUMBER() OVER (PARTITION BY ... ORDER BY ...)). It is used by
// the relationship loader to apply Include Limit(n) per parent rather than
// across the whole batch. When set, the emitter wraps the base SELECT and
// keeps only rows whose row number is <= Limit.
type PartitionLimit struct {
	PartitionBy string        // foreign-key column to partition by
	OrderBy     []OrderByExpr // ordering within each partition (defaults to PK at the call site)
	Limit       int           // rows kept per partition
}

type SelectNode struct {
	Table          string
	Columns        []string
	Where          *WhereClause
	OrderBy        []OrderByExpr
	Limit          *int
	Offset         *int
	Type           QueryType
	GroupBy        []string
	Having         *WhereClause
	Aggregates     []AggregateExpr
	PartitionLimit *PartitionLimit
}
