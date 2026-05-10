package ast_test

import (
	"testing"

	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
)

func TestSelectNode_Defaults(t *testing.T) {
	node := ast.SelectNode{
		Table:   "users",
		Columns: []string{"id", "name"},
		Type:    ast.QuerySelect,
	}
	assert.Equal(t, "users", node.Table)
	assert.Equal(t, []string{"id", "name"}, node.Columns)
	assert.Nil(t, node.Where)
	assert.Empty(t, node.OrderBy)
	assert.Nil(t, node.Limit)
	assert.Nil(t, node.Offset)
}

func TestWhereClause_Comparison(t *testing.T) {
	clause := ast.WhereClause{
		Comparison: &ast.ComparisonNode{
			Column: "age",
			Op:     ast.OpGTE,
			Value:  18,
		},
	}
	assert.Equal(t, "age", clause.Comparison.Column)
	assert.Equal(t, ast.OpGTE, clause.Comparison.Op)
	assert.Equal(t, 18, clause.Comparison.Value)
}

func TestWhereClause_LogicalAnd(t *testing.T) {
	clause := ast.WhereClause{
		LogicalOp: ast.LogicalAnd,
		Children: []ast.WhereClause{
			{Comparison: &ast.ComparisonNode{Column: "age", Op: ast.OpGTE, Value: 18}},
			{Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "admin"}},
		},
	}
	assert.Equal(t, ast.LogicalAnd, clause.LogicalOp)
	assert.Len(t, clause.Children, 2)
}

func TestOrderByExpr(t *testing.T) {
	expr := ast.OrderByExpr{Column: "name", Direction: ast.Desc}
	assert.Equal(t, "name", expr.Column)
	assert.Equal(t, ast.Desc, expr.Direction)
}
