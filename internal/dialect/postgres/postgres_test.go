package postgres

import (
	"testing"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
	"github.com/stretchr/testify/assert"
)

func intPtr(n int) *int { return &n }

func TestPostgres_BuildSelect(t *testing.T) {
	pg := New()

	tests := []struct {
		name     string
		node     ast.SelectNode
		expected dialect.Result
	}{
		{
			name: "simple select all columns",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name", "email"},
				Type:    ast.QuerySelect,
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name", "email" FROM "users"`,
				Args: nil,
			},
		},
		{
			name: "where eq",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "id",
						Op:     ast.OpEq,
						Value:  1,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "id" = $1`,
				Args: []any{1},
			},
		},
		{
			name: "where neq",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "role",
						Op:     ast.OpNEQ,
						Value:  "admin",
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "role" != $1`,
				Args: []any{"admin"},
			},
		},
		{
			name: "where gt",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "age",
						Op:     ast.OpGT,
						Value:  18,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "age" > $1`,
				Args: []any{18},
			},
		},
		{
			name: "where gte",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "age",
						Op:     ast.OpGTE,
						Value:  18,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "age" >= $1`,
				Args: []any{18},
			},
		},
		{
			name: "where lt",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "age",
						Op:     ast.OpLT,
						Value:  65,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "age" < $1`,
				Args: []any{65},
			},
		},
		{
			name: "where lte",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "age",
						Op:     ast.OpLTE,
						Value:  65,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "age" <= $1`,
				Args: []any{65},
			},
		},
		{
			name: "where like",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "name",
						Op:     ast.OpLike,
						Value:  "J%",
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "name" LIKE $1`,
				Args: []any{"J%"},
			},
		},
		{
			name: "where in",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "role",
						Op:     ast.OpIn,
						Values: []any{"admin", "mod"},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "role" IN ($1, $2)`,
				Args: []any{"admin", "mod"},
			},
		},
		{
			name: "where is null",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "deleted_at",
						Op:     ast.OpIsNull,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "deleted_at" IS NULL`,
				Args: nil,
			},
		},
		{
			name: "where is not null",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "deleted_at",
						Op:     ast.OpIsNotNull,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "deleted_at" IS NOT NULL`,
				Args: nil,
			},
		},
		{
			name: "where AND two conditions",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalAnd,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{
								Column: "age",
								Op:     ast.OpGTE,
								Value:  18,
							},
						},
						{
							Comparison: &ast.ComparisonNode{
								Column: "role",
								Op:     ast.OpEq,
								Value:  "admin",
							},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE ("age" >= $1 AND "role" = $2)`,
				Args: []any{18, "admin"},
			},
		},
		{
			name: "where OR two conditions",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalOr,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{
								Column: "role",
								Op:     ast.OpEq,
								Value:  "admin",
							},
						},
						{
							Comparison: &ast.ComparisonNode{
								Column: "age",
								Op:     ast.OpGTE,
								Value:  21,
							},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE ("role" = $1 OR "age" >= $2)`,
				Args: []any{"admin", 21},
			},
		},
		{
			name: "where NOT",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalNot,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{
								Column: "role",
								Op:     ast.OpEq,
								Value:  "banned",
							},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE NOT ("role" = $1)`,
				Args: []any{"banned"},
			},
		},
		{
			name: "order by single asc",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{
					{Column: "name", Direction: ast.Asc},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" ORDER BY "name"`,
				Args: nil,
			},
		},
		{
			name: "order by single desc",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{
					{Column: "created_at", Direction: ast.Desc},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" ORDER BY "created_at" DESC`,
				Args: nil,
			},
		},
		{
			name: "order by multiple",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{
					{Column: "name", Direction: ast.Asc},
					{Column: "age", Direction: ast.Desc},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" ORDER BY "name", "age" DESC`,
				Args: nil,
			},
		},
		{
			name: "limit only",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Limit:   intPtr(10),
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" LIMIT 10`,
				Args: nil,
			},
		},
		{
			name: "offset only",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Offset:  intPtr(20),
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" OFFSET 20`,
				Args: nil,
			},
		},
		{
			name: "limit and offset",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Limit:   intPtr(10),
				Offset:  intPtr(20),
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" LIMIT 10 OFFSET 20`,
				Args: nil,
			},
		},
		{
			name: "count",
			node: ast.SelectNode{
				Table: "users",
				Type:  ast.QueryCount,
			},
			expected: dialect.Result{
				SQL:  `SELECT COUNT(*) FROM "users"`,
				Args: nil,
			},
		},
		{
			name: "count with where",
			node: ast.SelectNode{
				Table: "users",
				Type:  ast.QueryCount,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "role",
						Op:     ast.OpEq,
						Value:  "admin",
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT COUNT(*) FROM "users" WHERE "role" = $1`,
				Args: []any{"admin"},
			},
		},
		{
			name: "exists",
			node: ast.SelectNode{
				Table: "users",
				Type:  ast.QueryExists,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "email",
						Op:     ast.OpEq,
						Value:  "test@test.com",
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT EXISTS(SELECT 1 FROM "users" WHERE "email" = $1)`,
				Args: []any{"test@test.com"},
			},
		},
		{
			name: "complex combined query",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name", "email"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalAnd,
					Children: []ast.WhereClause{
						{
							LogicalOp: ast.LogicalOr,
							Children: []ast.WhereClause{
								{
									Comparison: &ast.ComparisonNode{
										Column: "role",
										Op:     ast.OpEq,
										Value:  "admin",
									},
								},
								{
									Comparison: &ast.ComparisonNode{
										Column: "role",
										Op:     ast.OpEq,
										Value:  "mod",
									},
								},
							},
						},
						{
							Comparison: &ast.ComparisonNode{
								Column: "age",
								Op:     ast.OpGTE,
								Value:  18,
							},
						},
					},
				},
				OrderBy: []ast.OrderByExpr{
					{Column: "name", Direction: ast.Asc},
					{Column: "created_at", Direction: ast.Desc},
				},
				Limit:  intPtr(25),
				Offset: intPtr(50),
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name", "email" FROM "users" WHERE (("role" = $1 OR "role" = $2) AND "age" >= $3) ORDER BY "name", "created_at" DESC LIMIT 25 OFFSET 50`,
				Args: []any{"admin", "mod", 18},
			},
		},
		{
			name: "empty where (no conditions)",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users"`,
				Args: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pg.BuildSelect(tt.node)
			assert.Equal(t, tt.expected.SQL, result.SQL)
			assert.Equal(t, tt.expected.Args, result.Args)
		})
	}
}

func buildRawWhere(sql string, args ...any) dialect.Result {
	p := New()
	node := ast.SelectNode{
		Table:   "test",
		Columns: []string{"id"},
		Where: &ast.WhereClause{
			Raw:     &sql,
			RawArgs: args,
		},
		Type: ast.QuerySelect,
	}
	return p.BuildSelect(node)
}

func TestRawPlaceholder_Basic(t *testing.T) {
	r := buildRawWhere("age > ? AND name = ?", 18, "alice")
	assert.Equal(t, `SELECT "id" FROM "test" WHERE age > $1 AND name = $2`, r.SQL)
	assert.Equal(t, []any{18, "alice"}, r.Args)
}

func TestRawPlaceholder_InsideSingleQuotedString(t *testing.T) {
	r := buildRawWhere("name = '?' AND age > ?", 18)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE name = '?' AND age > $1`, r.SQL)
	assert.Equal(t, []any{18}, r.Args)
}

func TestRawPlaceholder_InsideDoubleQuotedIdentifier(t *testing.T) {
	r := buildRawWhere(`"col?" = ?`, 42)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE "col?" = $1`, r.SQL)
	assert.Equal(t, []any{42}, r.Args)
}

func TestRawPlaceholder_InsideDollarQuotedString(t *testing.T) {
	r := buildRawWhere("body = $$hello ? world$$ AND id = ?", 1)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE body = $$hello ? world$$ AND id = $1`, r.SQL)
	assert.Equal(t, []any{1}, r.Args)
}

func TestRawPlaceholder_EscapedSingleQuote(t *testing.T) {
	r := buildRawWhere("name = 'it''s a ?' AND id = ?", 1)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE name = 'it''s a ?' AND id = $1`, r.SQL)
	assert.Equal(t, []any{1}, r.Args)
}

func TestRawPlaceholder_MismatchReturnsError(t *testing.T) {
	p := New()
	sql := "a = ? AND b = ?"
	node := ast.SelectNode{
		Table:   "test",
		Columns: []string{"id"},
		Where: &ast.WhereClause{
			Raw:     &sql,
			RawArgs: []any{1}, // only 1 arg for 2 placeholders
		},
		Type: ast.QuerySelect,
	}
	r := p.BuildSelect(node)
	assert.Contains(t, r.SQL, "ERROR")
}
