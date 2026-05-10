package drel

import (
	"testing"

	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
)

func TestQueryBuilder_FiltersAppliedInBuildAST(t *testing.T) {
	meta := &ModelMeta[struct{}]{
		Table:   "users",
		Columns: []string{"id", "name"},
		Filters: []NamedFilter{
			{Name: "soft_delete", Clause: ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "deleted_at", Op: ast.OpIsNull},
			}},
		},
	}
	qb := newQueryBuilder[struct{}](nil, meta)
	node := qb.buildAST(ast.QuerySelect)

	assert.NotNil(t, node.Where)
	assert.NotNil(t, node.Where.Comparison)
	assert.Equal(t, "deleted_at", node.Where.Comparison.Column)
	assert.Equal(t, ast.OpIsNull, node.Where.Comparison.Op)
}

func TestQueryBuilder_FiltersAndUserWheresCombined(t *testing.T) {
	meta := &ModelMeta[struct{}]{
		Table:   "users",
		Columns: []string{"id", "name"},
		Filters: []NamedFilter{
			{Name: "soft_delete", Clause: ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "deleted_at", Op: ast.OpIsNull},
			}},
		},
	}
	qb := newQueryBuilder[struct{}](nil, meta).
		Where(newComparison("name", ast.OpEq, "Alice"))
	node := qb.buildAST(ast.QuerySelect)

	assert.NotNil(t, node.Where)
	assert.Equal(t, ast.LogicalAnd, node.Where.LogicalOp)
	assert.Len(t, node.Where.Children, 2)
}

func TestQueryBuilder_Unscoped_RemovesAllFilters(t *testing.T) {
	meta := &ModelMeta[struct{}]{
		Table:   "users",
		Columns: []string{"id", "name"},
		Filters: []NamedFilter{
			{Name: "soft_delete", Clause: ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "deleted_at", Op: ast.OpIsNull},
			}},
		},
	}
	qb := newQueryBuilder[struct{}](nil, meta).Unscoped()
	node := qb.buildAST(ast.QuerySelect)

	assert.Nil(t, node.Where)
}

func TestQueryBuilder_WithoutFilter_RemovesSpecificFilter(t *testing.T) {
	meta := &ModelMeta[struct{}]{
		Table:   "users",
		Columns: []string{"id", "name"},
		Filters: []NamedFilter{
			{Name: "soft_delete", Clause: ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "deleted_at", Op: ast.OpIsNull},
			}},
			{Name: "active", Clause: ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "active", Op: ast.OpEq, Value: true},
			}},
		},
	}
	qb := newQueryBuilder[struct{}](nil, meta).WithoutFilter("soft_delete")
	node := qb.buildAST(ast.QuerySelect)

	assert.NotNil(t, node.Where)
	assert.Equal(t, "active", node.Where.Comparison.Column)
}
