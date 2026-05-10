package drel

import "github.com/alternayte/drel/internal/ast"

type OrderExpr struct {
	column    string
	direction ast.Direction
}

func (o OrderExpr) ToAST() ast.OrderByExpr {
	return ast.OrderByExpr{
		Column:    o.column,
		Direction: o.direction,
	}
}
