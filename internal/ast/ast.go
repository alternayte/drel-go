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

type SelectNode struct {
	Table      string
	Columns    []string
	Where      *WhereClause
	OrderBy    []OrderByExpr
	Limit      *int
	Offset     *int
	Type       QueryType
	GroupBy    []string
	Having     *WhereClause
	Aggregates []AggregateExpr
}
