package drel

import "github.com/alternayte/drel/internal/ast"

type Predicate struct {
	clause ast.WhereClause
}

func newComparison(column string, op ast.Operator, value any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     op,
				Value:  value,
			},
		},
	}
}

func newInComparison(column string, values []any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpIn,
				Values: values,
			},
		},
	}
}

func newNullCheck(column string, isNull bool) Predicate {
	op := ast.OpIsNull
	if !isNull {
		op = ast.OpIsNotNull
	}
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     op,
			},
		},
	}
}

func And(preds ...Predicate) Predicate {
	children := make([]ast.WhereClause, len(preds))
	for i, p := range preds {
		children[i] = p.clause
	}
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalAnd,
			Children:  children,
		},
	}
}

func Or(preds ...Predicate) Predicate {
	children := make([]ast.WhereClause, len(preds))
	for i, p := range preds {
		children[i] = p.clause
	}
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalOr,
			Children:  children,
		},
	}
}

func Not(pred Predicate) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			LogicalOp: ast.LogicalNot,
			Children:  []ast.WhereClause{pred.clause},
		},
	}
}

func (p Predicate) ToAST() ast.WhereClause {
	return p.clause
}
