package drel

import (
	"fmt"

	"github.com/alternayte/drel/internal/ast"
)

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

func newNotInComparison(column string, values []any) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpNotIn,
				Values: values,
			},
		},
	}
}

func newBetweenComparison[T any](column string, low, high T) Predicate {
	return Predicate{
		clause: ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpBetween,
				Values: []any{low, high},
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

// Raw creates a predicate from a raw SQL expression with bound arguments.
// Use ? as placeholder for each argument; they are rewritten to $N for Postgres.
// Panics if the number of ? placeholders does not match the number of arguments.
func Raw(sql string, args ...any) Predicate {
	count := 0
	for _, c := range sql {
		if c == '?' {
			count++
		}
	}
	if count != len(args) {
		panic(fmt.Sprintf("drel.Raw: %d placeholder(s) but %d argument(s)", count, len(args)))
	}
	return Predicate{
		clause: ast.WhereClause{
			Raw:     &sql,
			RawArgs: args,
		},
	}
}

func (p Predicate) ToAST() ast.WhereClause {
	return p.clause
}
