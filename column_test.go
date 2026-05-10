package drel_test

import (
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
)

func TestColumn_Eq(t *testing.T) {
	col := drel.NewCol[string]("email")
	pred := col.Eq("test@example.com")
	clause := pred.ToAST()
	assert.Equal(t, "email", clause.Comparison.Column)
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
	assert.Equal(t, "test@example.com", clause.Comparison.Value)
}

func TestColumn_NEQ(t *testing.T) {
	col := drel.NewCol[string]("role")
	pred := col.NEQ("admin")
	clause := pred.ToAST()
	assert.Equal(t, ast.OpNEQ, clause.Comparison.Op)
}

func TestColumn_IsNull(t *testing.T) {
	col := drel.NewCol[string]("email")
	pred := col.IsNull()
	clause := pred.ToAST()
	assert.Equal(t, ast.OpIsNull, clause.Comparison.Op)
}

func TestColumn_IsNotNull(t *testing.T) {
	col := drel.NewCol[string]("email")
	pred := col.IsNotNull()
	clause := pred.ToAST()
	assert.Equal(t, ast.OpIsNotNull, clause.Comparison.Op)
}

func TestColumn_In(t *testing.T) {
	col := drel.NewCol[string]("role")
	pred := col.In("admin", "user", "mod")
	clause := pred.ToAST()
	assert.Equal(t, ast.OpIn, clause.Comparison.Op)
	assert.Equal(t, []any{"admin", "user", "mod"}, clause.Comparison.Values)
}

func TestColumn_Asc(t *testing.T) {
	col := drel.NewCol[string]("name")
	order := col.Asc()
	expr := order.ToAST()
	assert.Equal(t, "name", expr.Column)
	assert.Equal(t, ast.Asc, expr.Direction)
}

func TestColumn_Desc(t *testing.T) {
	col := drel.NewCol[string]("name")
	order := col.Desc()
	expr := order.ToAST()
	assert.Equal(t, ast.Desc, expr.Direction)
}

func TestOrderedColumn_GT(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	pred := col.GT(18)
	clause := pred.ToAST()
	assert.Equal(t, ast.OpGT, clause.Comparison.Op)
	assert.Equal(t, 18, clause.Comparison.Value)
}

func TestOrderedColumn_GTE(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	pred := col.GTE(18)
	clause := pred.ToAST()
	assert.Equal(t, ast.OpGTE, clause.Comparison.Op)
}

func TestOrderedColumn_LT(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	pred := col.LT(65)
	clause := pred.ToAST()
	assert.Equal(t, ast.OpLT, clause.Comparison.Op)
}

func TestOrderedColumn_LTE(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	pred := col.LTE(65)
	clause := pred.ToAST()
	assert.Equal(t, ast.OpLTE, clause.Comparison.Op)
}

func TestOrderedColumn_DelegatesEq(t *testing.T) {
	col := drel.NewOrderedCol[int]("age")
	pred := col.Eq(25)
	clause := pred.ToAST()
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
	assert.Equal(t, 25, clause.Comparison.Value)
}

func TestStringColumn_Like(t *testing.T) {
	col := drel.NewStringCol("name")
	pred := col.Like("J%")
	clause := pred.ToAST()
	assert.Equal(t, ast.OpLike, clause.Comparison.Op)
	assert.Equal(t, "J%", clause.Comparison.Value)
}

func TestStringColumn_Contains(t *testing.T) {
	col := drel.NewStringCol("name")
	pred := col.Contains("oh")
	clause := pred.ToAST()
	assert.Equal(t, ast.OpLike, clause.Comparison.Op)
	assert.Equal(t, "%oh%", clause.Comparison.Value)
}

func TestStringColumn_HasPrefix(t *testing.T) {
	col := drel.NewStringCol("name")
	pred := col.HasPrefix("Jo")
	clause := pred.ToAST()
	assert.Equal(t, "Jo%", clause.Comparison.Value)
}

func TestStringColumn_HasSuffix(t *testing.T) {
	col := drel.NewStringCol("name")
	pred := col.HasSuffix("hn")
	clause := pred.ToAST()
	assert.Equal(t, "%hn", clause.Comparison.Value)
}

func TestBoolColumn_IsTrue(t *testing.T) {
	col := drel.NewBoolCol("active")
	pred := col.IsTrue()
	clause := pred.ToAST()
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
	assert.Equal(t, true, clause.Comparison.Value)
}

func TestBoolColumn_IsFalse(t *testing.T) {
	col := drel.NewBoolCol("active")
	pred := col.IsFalse()
	clause := pred.ToAST()
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
	assert.Equal(t, false, clause.Comparison.Value)
}
