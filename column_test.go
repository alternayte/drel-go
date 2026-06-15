package drel_test

import (
	"testing"
	"time"

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

func TestTimeColumn_GT(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clause := col.GT(ts).ToAST()
	assert.Equal(t, "created_at", clause.Comparison.Column)
	assert.Equal(t, ast.OpGT, clause.Comparison.Op)
	assert.Equal(t, ts, clause.Comparison.Value)
}

func TestTimeColumn_GTE(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clause := col.GTE(ts).ToAST()
	assert.Equal(t, ast.OpGTE, clause.Comparison.Op)
}

func TestTimeColumn_LT(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	clause := col.LT(ts).ToAST()
	assert.Equal(t, ast.OpLT, clause.Comparison.Op)
}

func TestTimeColumn_LTE(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	clause := col.LTE(ts).ToAST()
	assert.Equal(t, ast.OpLTE, clause.Comparison.Op)
}

func TestTimeColumn_Between(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	lo := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	hi := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	clause := col.Between(lo, hi).ToAST()
	assert.Equal(t, ast.OpBetween, clause.Comparison.Op)
	assert.Equal(t, lo, clause.Comparison.Values[0])
	assert.Equal(t, hi, clause.Comparison.Values[1])
}

func TestTimeColumn_Before(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clause := col.Before(ts).ToAST()
	// Before is an alias for LT.
	assert.Equal(t, ast.OpLT, clause.Comparison.Op)
	assert.Equal(t, ts, clause.Comparison.Value)
}

func TestTimeColumn_After(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clause := col.After(ts).ToAST()
	// After is an alias for GT.
	assert.Equal(t, ast.OpGT, clause.Comparison.Op)
	assert.Equal(t, ts, clause.Comparison.Value)
}

func TestTimeColumn_Eq(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	clause := col.Eq(ts).ToAST()
	assert.Equal(t, ast.OpEq, clause.Comparison.Op)
	assert.Equal(t, ts, clause.Comparison.Value)
}

func TestTimeColumn_In(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	ts1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clause := col.In(ts1, ts2).ToAST()
	assert.Equal(t, ast.OpIn, clause.Comparison.Op)
	assert.Len(t, clause.Comparison.Values, 2)
}

func TestTimeColumn_Asc(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	expr := col.Asc().ToAST()
	assert.Equal(t, "created_at", expr.Column)
	assert.Equal(t, ast.Asc, expr.Direction)
}

func TestTimeColumn_Desc(t *testing.T) {
	col := drel.NewTimeCol("created_at")
	expr := col.Desc().ToAST()
	assert.Equal(t, ast.Desc, expr.Direction)
}

func TestComparableColumn_GT(t *testing.T) {
	col := drel.NewComparableCol[int64]("score")
	clause := col.GT(int64(100)).ToAST()
	assert.Equal(t, "score", clause.Comparison.Column)
	assert.Equal(t, ast.OpGT, clause.Comparison.Op)
	assert.Equal(t, int64(100), clause.Comparison.Value)
}

func TestComparableColumn_Between(t *testing.T) {
	col := drel.NewComparableCol[int64]("score")
	clause := col.Between(int64(10), int64(99)).ToAST()
	assert.Equal(t, ast.OpBetween, clause.Comparison.Op)
	assert.Equal(t, int64(10), clause.Comparison.Values[0])
	assert.Equal(t, int64(99), clause.Comparison.Values[1])
}

func TestComparableColumn_LTE(t *testing.T) {
	col := drel.NewComparableCol[int64]("score")
	clause := col.LTE(int64(50)).ToAST()
	assert.Equal(t, ast.OpLTE, clause.Comparison.Op)
}

func TestComparableColumn_In(t *testing.T) {
	col := drel.NewComparableCol[string]("status")
	clause := col.In("a", "b", "c").ToAST()
	assert.Equal(t, ast.OpIn, clause.Comparison.Op)
	assert.Len(t, clause.Comparison.Values, 3)
}
