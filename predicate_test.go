package drel_test

import (
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnd_CombinesTwoPredicates(t *testing.T) {
	p1 := drel.NewCol[int]("age").Eq(18)
	p2 := drel.NewCol[string]("role").Eq("admin")
	combined := drel.And(p1, p2)

	clause := combined.ToAST()
	assert.Equal(t, ast.LogicalAnd, clause.LogicalOp)
	assert.Len(t, clause.Children, 2)
}

func TestOr_CombinesTwoPredicates(t *testing.T) {
	p1 := drel.NewCol[int]("age").Eq(18)
	p2 := drel.NewCol[int]("age").Eq(21)
	combined := drel.Or(p1, p2)

	clause := combined.ToAST()
	assert.Equal(t, ast.LogicalOr, clause.LogicalOp)
	assert.Len(t, clause.Children, 2)
}

func TestNot_WrapsOnePredicate(t *testing.T) {
	p := drel.NewCol[int]("age").Eq(18)
	negated := drel.Not(p)

	clause := negated.ToAST()
	assert.Equal(t, ast.LogicalNot, clause.LogicalOp)
	require.Len(t, clause.Children, 1)
}

func TestTrue_IsEmptyNoOpClause(t *testing.T) {
	clause := drel.True().ToAST()
	assert.Nil(t, clause.Comparison)
	assert.Nil(t, clause.Raw)
	assert.Nil(t, clause.Children)
}

func TestWhereIf_ReturnsPredWhenTrue(t *testing.T) {
	p := drel.NewCol[int]("age").Eq(18)
	got := drel.WhereIf(true, p)
	clause := got.ToAST()
	require.NotNil(t, clause.Comparison)
	assert.Equal(t, "age", clause.Comparison.Column)
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
}

func TestWhereIf_ReturnsNoOpWhenFalse(t *testing.T) {
	p := drel.NewCol[int]("age").Eq(18)
	got := drel.WhereIf(false, p)
	clause := got.ToAST()
	assert.Nil(t, clause.Comparison)
	assert.Nil(t, clause.Raw)
	assert.Nil(t, clause.Children)
}
