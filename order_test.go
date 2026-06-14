package drel_test

import (
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
)

func TestOrderExpr_NullsFirstLast_ToAST(t *testing.T) {
	asc := drel.NewOrderedCol[int]("rank").Asc()
	assert.Equal(t, ast.NullsDefault, asc.ToAST().Nulls)

	first := drel.NewOrderedCol[int]("rank").Asc().NullsFirst()
	got := first.ToAST()
	assert.Equal(t, "rank", got.Column)
	assert.Equal(t, ast.Asc, got.Direction)
	assert.Equal(t, ast.NullsFirst, got.Nulls)

	last := drel.NewStringCol("name").Desc().NullsLast()
	gotLast := last.ToAST()
	assert.Equal(t, "name", gotLast.Column)
	assert.Equal(t, ast.Desc, gotLast.Direction)
	assert.Equal(t, ast.NullsLast, gotLast.Nulls)
}
