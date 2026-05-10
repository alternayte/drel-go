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
	OpIn
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
}

type OrderByExpr struct {
	Column    string
	Direction Direction
}

type SelectNode struct {
	Table   string
	Columns []string
	Where   *WhereClause
	OrderBy []OrderByExpr
	Limit   *int
	Offset  *int
	Type    QueryType
}
