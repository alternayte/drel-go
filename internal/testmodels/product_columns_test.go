package testmodels

import (
	"testing"
	"time"

	"github.com/alternayte/drel/internal/ast"
	"github.com/stretchr/testify/assert"
)

func TestProducts_CreatedAtRangeOps(t *testing.T) {
	since := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	clause := Products.CreatedAt.GT(since).ToAST()
	assert.Equal(t, "created_at", clause.Comparison.Column)
	assert.Equal(t, ast.OpGT, clause.Comparison.Op)

	low := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	high := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	b := Products.CreatedAt.Between(low, high).ToAST()
	assert.Equal(t, ast.OpBetween, b.Comparison.Op)
	assert.Equal(t, []any{low, high}, b.Comparison.Values)
}
