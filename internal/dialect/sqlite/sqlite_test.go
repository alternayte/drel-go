package sqlite

import (
	"testing"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
	"github.com/stretchr/testify/assert"
)

func intPtr(n int) *int { return &n }

// ─── SupportsReturning ────────────────────────────────────────────────────────

func TestSQLite_SupportsReturning(t *testing.T) {
	assert.False(t, New().SupportsReturning())
}

// ─── Now ─────────────────────────────────────────────────────────────────────

func TestSQLite_Now(t *testing.T) {
	assert.Equal(t, "CURRENT_TIMESTAMP", New().Now())
}

// ─── BuildSelect ─────────────────────────────────────────────────────────────

func TestSQLite_BuildSelect(t *testing.T) {
	s := New()

	tests := []struct {
		name     string
		node     ast.SelectNode
		expected dialect.Result
	}{
		{
			name: "simple select no where",
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
			name: "where eq with ? placeholder",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id", "name"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "id",
						Op:     ast.OpEq,
						Value:  42,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id", "name" FROM "users" WHERE "id" = ?`,
				Args: []any{42},
			},
		},
		{
			name: "where neq",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "role" != ?`,
				Args: []any{"admin"},
			},
		},
		{
			name: "where gt",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "age" > ?`,
				Args: []any{18},
			},
		},
		{
			name: "where gte",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "age" >= ?`,
				Args: []any{18},
			},
		},
		{
			name: "where lt",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "age" < ?`,
				Args: []any{65},
			},
		},
		{
			name: "where lte",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "age" <= ?`,
				Args: []any{65},
			},
		},
		{
			name: "where like",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
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
				SQL:  `SELECT "id" FROM "users" WHERE "name" LIKE ?`,
				Args: []any{"J%"},
			},
		},
		{
			name: "ilike maps to like",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "name",
						Op:     ast.OpILike,
						Value:  "j%",
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE "name" LIKE ?`,
				Args: []any{"j%"},
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
				SQL:  `SELECT "id", "name" FROM "users" WHERE "role" IN (?, ?)`,
				Args: []any{"admin", "mod"},
			},
		},
		{
			name: "where not in",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "status",
						Op:     ast.OpNotIn,
						Values: []any{"banned", "pending"},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE "status" NOT IN (?, ?)`,
				Args: []any{"banned", "pending"},
			},
		},
		{
			name: "where between",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "age",
						Op:     ast.OpBetween,
						Values: []any{18, 65},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE "age" BETWEEN ? AND ?`,
				Args: []any{18, 65},
			},
		},
		{
			name: "where is null",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "deleted_at",
						Op:     ast.OpIsNull,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE "deleted_at" IS NULL`,
				Args: nil,
			},
		},
		{
			name: "where is not null",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{
						Column: "deleted_at",
						Op:     ast.OpIsNotNull,
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE "deleted_at" IS NOT NULL`,
				Args: nil,
			},
		},
		{
			name: "where AND",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalAnd,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{Column: "age", Op: ast.OpGTE, Value: 18},
						},
						{
							Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "admin"},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE ("age" >= ? AND "role" = ?)`,
				Args: []any{18, "admin"},
			},
		},
		{
			name: "where OR",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalOr,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "admin"},
						},
						{
							Comparison: &ast.ComparisonNode{Column: "age", Op: ast.OpGTE, Value: 21},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE ("role" = ? OR "age" >= ?)`,
				Args: []any{"admin", 21},
			},
		},
		{
			name: "where NOT",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Where: &ast.WhereClause{
					LogicalOp: ast.LogicalNot,
					Children: []ast.WhereClause{
						{
							Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "banned"},
						},
					},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" WHERE NOT ("role" = ?)`,
				Args: []any{"banned"},
			},
		},
		{
			name: "order by asc",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{{Column: "name", Direction: ast.Asc}},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" ORDER BY "name"`,
				Args: nil,
			},
		},
		{
			name: "order by desc",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{{Column: "created_at", Direction: ast.Desc}},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" ORDER BY "created_at" DESC`,
				Args: nil,
			},
		},
		{
			name: "order by asc nulls last",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{
					{Column: "name", Direction: ast.Asc, Nulls: ast.NullsLast},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" ORDER BY "name" NULLS LAST`,
				Args: nil,
			},
		},
		{
			name: "order by desc nulls first",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				OrderBy: []ast.OrderByExpr{
					{Column: "rank", Direction: ast.Desc, Nulls: ast.NullsFirst},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" ORDER BY "rank" DESC NULLS FIRST`,
				Args: nil,
			},
		},
		{
			name: "limit and offset",
			node: ast.SelectNode{
				Table:   "users",
				Columns: []string{"id"},
				Type:    ast.QuerySelect,
				Limit:   intPtr(10),
				Offset:  intPtr(20),
			},
			expected: dialect.Result{
				SQL:  `SELECT "id" FROM "users" LIMIT 10 OFFSET 20`,
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
					Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "admin"},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT COUNT(*) FROM "users" WHERE "role" = ?`,
				Args: []any{"admin"},
			},
		},
		{
			name: "exists",
			node: ast.SelectNode{
				Table: "users",
				Type:  ast.QueryExists,
				Where: &ast.WhereClause{
					Comparison: &ast.ComparisonNode{Column: "email", Op: ast.OpEq, Value: "test@example.com"},
				},
			},
			expected: dialect.Result{
				SQL:  `SELECT EXISTS(SELECT 1 FROM "users" WHERE "email" = ?)`,
				Args: []any{"test@example.com"},
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
								{Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "admin"}},
								{Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "mod"}},
							},
						},
						{Comparison: &ast.ComparisonNode{Column: "age", Op: ast.OpGTE, Value: 18}},
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
				SQL:  `SELECT "id", "name", "email" FROM "users" WHERE (("role" = ? OR "role" = ?) AND "age" >= ?) ORDER BY "name", "created_at" DESC LIMIT 25 OFFSET 50`,
				Args: []any{"admin", "mod", 18},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.BuildSelect(tt.node)
			assert.Equal(t, tt.expected.SQL, result.SQL)
			assert.Equal(t, tt.expected.Args, result.Args)
		})
	}
}

// ─── Raw predicates ───────────────────────────────────────────────────────────

func buildRawWhere(sql string, args ...any) dialect.Result {
	s := New()
	node := ast.SelectNode{
		Table:   "test",
		Columns: []string{"id"},
		Where: &ast.WhereClause{
			Raw:     &sql,
			RawArgs: args,
		},
		Type: ast.QuerySelect,
	}
	return s.BuildSelect(node)
}

func TestSQLite_RawPlaceholder_Basic(t *testing.T) {
	r := buildRawWhere("age > ? AND name = ?", 18, "alice")
	assert.Equal(t, `SELECT "id" FROM "test" WHERE age > ? AND name = ?`, r.SQL)
	assert.Equal(t, []any{18, "alice"}, r.Args)
}

func TestSQLite_RawPlaceholder_InsideSingleQuotedString(t *testing.T) {
	r := buildRawWhere("name = '?' AND age > ?", 18)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE name = '?' AND age > ?`, r.SQL)
	assert.Equal(t, []any{18}, r.Args)
}

func TestSQLite_RawPlaceholder_InsideDoubleQuotedIdentifier(t *testing.T) {
	r := buildRawWhere(`"col?" = ?`, 42)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE "col?" = ?`, r.SQL)
	assert.Equal(t, []any{42}, r.Args)
}

func TestSQLite_RawPlaceholder_EscapedSingleQuote(t *testing.T) {
	r := buildRawWhere("name = 'it''s a ?' AND id = ?", 1)
	assert.Equal(t, `SELECT "id" FROM "test" WHERE name = 'it''s a ?' AND id = ?`, r.SQL)
	assert.Equal(t, []any{1}, r.Args)
}

func TestSQLite_RawPlaceholder_MismatchReturnsError(t *testing.T) {
	s := New()
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
	r := s.BuildSelect(node)
	assert.Contains(t, r.SQL, "ERROR")
}

// ─── BuildInsert ─────────────────────────────────────────────────────────────

func TestSQLite_BuildInsert(t *testing.T) {
	s := New()

	t.Run("basic insert no returning", func(t *testing.T) {
		r := s.BuildInsert("users", []string{"name", "email"}, []any{"Alice", "alice@example.com"}, nil)
		assert.Equal(t, `INSERT INTO "users" ("name", "email") VALUES (?, ?)`, r.SQL)
		assert.Equal(t, []any{"Alice", "alice@example.com"}, r.Args)
	})

	t.Run("returningCols ignored", func(t *testing.T) {
		r := s.BuildInsert("users", []string{"name"}, []any{"Bob"}, []string{"id", "created_at"})
		assert.Equal(t, `INSERT INTO "users" ("name") VALUES (?)`, r.SQL)
		assert.NotContains(t, r.SQL, "RETURNING")
	})
}

// ─── BuildUpdate ─────────────────────────────────────────────────────────────

func TestSQLite_BuildUpdate(t *testing.T) {
	s := New()

	t.Run("basic update", func(t *testing.T) {
		r := s.BuildUpdate("users",
			[]dialect.ColumnValue{
				{Column: "name", Value: "Alice"},
				{Column: "email", Value: "alice@example.com"},
			},
			"id", 1,
		)
		assert.Equal(t, `UPDATE "users" SET "name" = ?, "email" = ? WHERE "id" = ?`, r.SQL)
		assert.Equal(t, []any{"Alice", "alice@example.com", 1}, r.Args)
	})

	t.Run("update with raw expr", func(t *testing.T) {
		r := s.BuildUpdate("products",
			[]dialect.ColumnValue{
				{Column: "updated_at", Value: dialect.RawExpr{SQL: "CURRENT_TIMESTAMP"}},
				{Column: "stock", Value: 99},
			},
			"id", 5,
		)
		assert.Equal(t, `UPDATE "products" SET "updated_at" = CURRENT_TIMESTAMP, "stock" = ? WHERE "id" = ?`, r.SQL)
		assert.Equal(t, []any{99, 5}, r.Args)
	})
}

// ─── BuildDelete ─────────────────────────────────────────────────────────────

func TestSQLite_BuildDelete(t *testing.T) {
	s := New()
	r := s.BuildDelete("users", "id", 42)
	assert.Equal(t, `DELETE FROM "users" WHERE "id" = ?`, r.SQL)
	assert.Equal(t, []any{42}, r.Args)
}

// ─── BuildSoftDelete ─────────────────────────────────────────────────────────

func TestSQLite_BuildSoftDelete(t *testing.T) {
	s := New()
	r := s.BuildSoftDelete("users", "id", 7)
	assert.Equal(t, `UPDATE "users" SET "deleted_at" = CURRENT_TIMESTAMP WHERE "id" = ?`, r.SQL)
	assert.Equal(t, []any{7}, r.Args)
	assert.NotContains(t, r.SQL, "NOW()")
}

// ─── BuildUpdateVersioned ────────────────────────────────────────────────────

func TestSQLite_BuildUpdateVersioned(t *testing.T) {
	s := New()
	r := s.BuildUpdateVersioned("items",
		[]dialect.ColumnValue{{Column: "name", Value: "Widget"}},
		"id", 3, "version", 2,
	)
	assert.Equal(t,
		`UPDATE "items" SET "name" = ?, "version" = "version" + 1 WHERE "id" = ? AND "version" = ?`,
		r.SQL,
	)
	assert.Equal(t, []any{"Widget", 3, 2}, r.Args)
	assert.NotContains(t, r.SQL, "RETURNING")
}

// ─── BuildBulkInsert ─────────────────────────────────────────────────────────

func TestSQLite_BuildBulkInsert(t *testing.T) {
	s := New()
	r := s.BuildBulkInsert("users",
		[]string{"name", "email"},
		[][]any{
			{"Alice", "alice@example.com"},
			{"Bob", "bob@example.com"},
		},
	)
	assert.Equal(t,
		`INSERT INTO "users" ("name", "email") VALUES (?, ?), (?, ?)`,
		r.SQL,
	)
	assert.Equal(t, []any{"Alice", "alice@example.com", "Bob", "bob@example.com"}, r.Args)
}

// ─── BuildBulkUpdate ─────────────────────────────────────────────────────────

func TestSQLite_BuildBulkUpdate(t *testing.T) {
	s := New()

	t.Run("with where", func(t *testing.T) {
		role := "member"
		r := s.BuildBulkUpdate("users",
			[]dialect.ColumnValue{{Column: "active", Value: true}},
			&ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: role},
			},
		)
		assert.Equal(t, `UPDATE "users" SET "active" = ? WHERE "role" = ?`, r.SQL)
		assert.Equal(t, []any{true, "member"}, r.Args)
	})

	t.Run("no where", func(t *testing.T) {
		r := s.BuildBulkUpdate("users",
			[]dialect.ColumnValue{{Column: "active", Value: false}},
			nil,
		)
		assert.Equal(t, `UPDATE "users" SET "active" = ?`, r.SQL)
		assert.Equal(t, []any{false}, r.Args)
	})
}

// ─── BuildBulkDelete ─────────────────────────────────────────────────────────

func TestSQLite_BuildBulkDelete(t *testing.T) {
	s := New()

	t.Run("with where", func(t *testing.T) {
		r := s.BuildBulkDelete("sessions",
			&ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "expired", Op: ast.OpEq, Value: true},
			},
		)
		assert.Equal(t, `DELETE FROM "sessions" WHERE "expired" = ?`, r.SQL)
		assert.Equal(t, []any{true}, r.Args)
	})

	t.Run("no where", func(t *testing.T) {
		r := s.BuildBulkDelete("sessions", nil)
		assert.Equal(t, `DELETE FROM "sessions"`, r.SQL)
		assert.Nil(t, r.Args)
	})
}

// ─── BuildBulkSoftDelete ─────────────────────────────────────────────────────

func TestSQLite_BuildBulkSoftDelete(t *testing.T) {
	s := New()

	t.Run("with where", func(t *testing.T) {
		r := s.BuildBulkSoftDelete("users",
			&ast.WhereClause{
				Comparison: &ast.ComparisonNode{Column: "role", Op: ast.OpEq, Value: "guest"},
			},
		)
		assert.Equal(t,
			`UPDATE "users" SET "deleted_at" = CURRENT_TIMESTAMP WHERE "deleted_at" IS NULL AND "role" = ?`,
			r.SQL,
		)
		assert.Equal(t, []any{"guest"}, r.Args)
		assert.NotContains(t, r.SQL, "NOW()")
	})

	t.Run("no where", func(t *testing.T) {
		r := s.BuildBulkSoftDelete("users", nil)
		assert.Equal(t,
			`UPDATE "users" SET "deleted_at" = CURRENT_TIMESTAMP WHERE "deleted_at" IS NULL`,
			r.SQL,
		)
		assert.Nil(t, r.Args)
	})
}

// ─── BuildBulkUpsert ─────────────────────────────────────────────────────────

func TestSQLite_BuildBulkUpsert(t *testing.T) {
	s := New()

	t.Run("do update", func(t *testing.T) {
		r := s.BuildBulkUpsert("users",
			[]string{"id", "name", "email"},
			[][]any{
				{1, "Alice", "alice@example.com"},
				{2, "Bob", "bob@example.com"},
			},
			[]string{"id"},
			[]string{"name", "email"},
			false,
		)
		assert.Equal(t,
			`INSERT INTO "users" ("id", "name", "email") VALUES (?, ?, ?), (?, ?, ?) ON CONFLICT ("id") DO UPDATE SET "name" = EXCLUDED."name", "email" = EXCLUDED."email"`,
			r.SQL,
		)
		assert.Equal(t, []any{1, "Alice", "alice@example.com", 2, "Bob", "bob@example.com"}, r.Args)
	})

	t.Run("do nothing", func(t *testing.T) {
		r := s.BuildBulkUpsert("users",
			[]string{"id", "name"},
			[][]any{{1, "Alice"}},
			[]string{"id"},
			nil,
			true,
		)
		assert.Equal(t,
			`INSERT INTO "users" ("id", "name") VALUES (?, ?) ON CONFLICT ("id") DO NOTHING`,
			r.SQL,
		)
		assert.Equal(t, []any{1, "Alice"}, r.Args)
	})
}

// ─── GroupBy / Having / Aggregates ───────────────────────────────────────────

func TestSQLite_BuildSelectGroupBy(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table:   "orders",
		Columns: []string{"category"},
		GroupBy: []string{"category"},
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggSum, Column: "amount", Alias: "total"},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t, `SELECT "category", SUM("amount") AS "total" FROM "orders" GROUP BY "category"`, result.SQL)
	assert.Nil(t, result.Args)
}

func TestSQLite_BuildSelectHaving(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table:   "orders",
		Columns: []string{"category"},
		GroupBy: []string{"category"},
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggCount, Column: "id", Alias: "cnt"},
		},
		Having: &ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: "cnt",
				Op:     ast.OpGT,
				Value:  5,
			},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t, `SELECT "category", COUNT("id") AS "cnt" FROM "orders" GROUP BY "category" HAVING "cnt" > ?`, result.SQL)
	assert.Equal(t, []any{5}, result.Args)
}

func TestSQLite_BuildSelectAggregateOnly(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table: "orders",
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggSum, Column: "amount", Alias: "total"},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t, `SELECT SUM("amount") AS "total" FROM "orders"`, result.SQL)
	assert.Nil(t, result.Args)
}

func TestSQLite_BuildSelectAggregateNoAlias(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table: "orders",
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggCount, Column: "id"},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t, `SELECT COUNT("id") FROM "orders"`, result.SQL)
	assert.Nil(t, result.Args)
}

func TestSQLite_BuildSelectAllAggFuncs(t *testing.T) {
	s := New()
	tests := []struct {
		fn       ast.AggFunc
		expected string
	}{
		{ast.AggSum, "SUM"},
		{ast.AggAvg, "AVG"},
		{ast.AggMin, "MIN"},
		{ast.AggMax, "MAX"},
		{ast.AggCount, "COUNT"},
	}
	for _, tt := range tests {
		node := ast.SelectNode{
			Table:      "t",
			Aggregates: []ast.AggregateExpr{{Func: tt.fn, Column: "v", Alias: "r"}},
			Type:       ast.QuerySelect,
		}
		result := s.BuildSelect(node)
		assert.Contains(t, result.SQL, tt.expected+"(")
	}
}

func TestSQLite_BuildSelectMultipleGroupByColumns(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table:   "sales",
		Columns: []string{"region", "category"},
		GroupBy: []string{"region", "category"},
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggSum, Column: "revenue", Alias: "total_revenue"},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t,
		`SELECT "region", "category", SUM("revenue") AS "total_revenue" FROM "sales" GROUP BY "region", "category"`,
		result.SQL,
	)
	assert.Nil(t, result.Args)
}

func TestSQLite_BuildSelectGroupByWithWhere(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table:   "orders",
		Columns: []string{"status"},
		GroupBy: []string{"status"},
		Aggregates: []ast.AggregateExpr{
			{Func: ast.AggCount, Column: "id", Alias: "cnt"},
		},
		Where: &ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: "created_at",
				Op:     ast.OpGTE,
				Value:  "2024-01-01",
			},
		},
		Type: ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t,
		`SELECT "status", COUNT("id") AS "cnt" FROM "orders" WHERE "created_at" >= ? GROUP BY "status"`,
		result.SQL,
	)
	assert.Equal(t, []any{"2024-01-01"}, result.Args)
}

func TestSQLite_BuildUpdate_DeduplicatesColumns(t *testing.T) {
	s := New()
	res := s.BuildUpdate("a_products",
		[]dialect.ColumnValue{
			{Column: "name", Value: "x"},
			{Column: "updated_by", Value: "alice"},
			{Column: "updated_by", Value: "bob"},
		},
		"id", 5)
	assert.Equal(t,
		`UPDATE "a_products" SET "name" = ?, "updated_by" = ? WHERE "id" = ?`,
		res.SQL)
	assert.Equal(t, []any{"x", "bob", 5}, res.Args)
}

func TestSQLite_BuildUpdateVersioned_DeduplicatesColumns(t *testing.T) {
	s := New()
	res := s.BuildUpdateVersioned("a_products",
		[]dialect.ColumnValue{
			{Column: "name", Value: "x"},
			{Column: "updated_by", Value: "alice"},
			{Column: "updated_by", Value: "bob"},
		},
		"id", 5, "version", 2)
	assert.Equal(t,
		`UPDATE "a_products" SET "name" = ?, "updated_by" = ?, "version" = "version" + 1 WHERE "id" = ? AND "version" = ?`,
		res.SQL)
	assert.Equal(t, []any{"x", "bob", 5, 2}, res.Args)
}

func TestSQLite_BuildDeleteVersioned(t *testing.T) {
	s := New()
	res := s.BuildDeleteVersioned("v_products", "id", 7, "version", 3)
	assert.Equal(t,
		`DELETE FROM "v_products" WHERE "id" = ? AND "version" = ?`,
		res.SQL)
	assert.Equal(t, []any{7, 3}, res.Args)
}

func TestSQLite_BuildSoftDeleteVersioned(t *testing.T) {
	s := New()
	res := s.BuildSoftDeleteVersioned("v_products", "id", 7, "version", 3)
	assert.Equal(t,
		`UPDATE "v_products" SET "deleted_at" = CURRENT_TIMESTAMP, "version" = "version" + 1 WHERE "id" = ? AND "version" = ?`,
		res.SQL)
	assert.Equal(t, []any{7, 3}, res.Args)
}

// ─── Interface compliance ─────────────────────────────────────────────────────

func TestSQLite_ImplementsDialect(t *testing.T) {
	var _ dialect.Dialect = New()
}

func TestSQLite_BuildSelect_PartitionLimit(t *testing.T) {
	d := New()
	node := ast.SelectNode{
		Table:   "posts",
		Columns: []string{"id", "author_id", "title"},
		Type:    ast.QuerySelect,
		Where: &ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: "author_id",
				Op:     ast.OpIn,
				Values: []any{1, 2},
			},
		},
		PartitionLimit: &ast.PartitionLimit{
			PartitionBy: "author_id",
			OrderBy:     []ast.OrderByExpr{{Column: "created_at", Direction: ast.Desc}},
			Limit:       3,
		},
	}
	result := d.BuildSelect(node)
	want := `SELECT "id", "author_id", "title" FROM (SELECT "id", "author_id", "title", ROW_NUMBER() OVER (PARTITION BY "author_id" ORDER BY "created_at" DESC) AS "_drel_rn" FROM "posts" WHERE "author_id" IN (?, ?)) AS "_drel_w" WHERE "_drel_rn" <= 3`
	assert.Equal(t, want, result.SQL)
	assert.Equal(t, []any{1, 2}, result.Args)
}

func TestSQLite_AdvisoryLockSQL_Unsupported(t *testing.T) {
	s := New()
	res, supported := s.AdvisoryLockSQL(42, dialect.AdvisoryLockBlocking)
	assert.False(t, supported)
	assert.Equal(t, dialect.Result{}, res)

	res, supported = s.AdvisoryLockSQL(42, dialect.AdvisoryLockTry)
	assert.False(t, supported)
	assert.Equal(t, dialect.Result{}, res)
}

func TestSQLite_BuildSelectDistinct(t *testing.T) {
	s := New()
	node := ast.SelectNode{
		Table:    "users",
		Columns:  []string{"city"},
		Distinct: true,
		Type:     ast.QuerySelect,
	}
	result := s.BuildSelect(node)
	assert.Equal(t, `SELECT DISTINCT "city" FROM "users"`, result.SQL)
	assert.Nil(t, result.Args)
}
