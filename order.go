package drel

import "github.com/alternayte/drel/internal/ast"

type OrderExpr struct {
	column    string
	direction ast.Direction
	nulls     ast.NullsOrder
}

func (o OrderExpr) ToAST() ast.OrderByExpr {
	return ast.OrderByExpr{
		Column:    o.column,
		Direction: o.direction,
		Nulls:     o.nulls,
	}
}

// NullsFirst orders NULL values before non-NULL values. Chain after Asc()/Desc().
// Pin this explicitly when paging over a nullable column so NULL rows are not
// silently dropped from cursor pages.
func (o OrderExpr) NullsFirst() OrderExpr {
	o.nulls = ast.NullsFirst
	return o
}

// NullsLast orders NULL values after non-NULL values. Chain after Asc()/Desc().
func (o OrderExpr) NullsLast() OrderExpr {
	o.nulls = ast.NullsLast
	return o
}
