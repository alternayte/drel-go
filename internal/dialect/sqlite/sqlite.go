package sqlite

import (
	"fmt"
	"strings"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
)

type SQLite struct{}

func New() *SQLite { return &SQLite{} }

func (s *SQLite) SupportsReturning() bool { return false }

func (s *SQLite) Now() string { return "CURRENT_TIMESTAMP" }

func (s *SQLite) BuildSelect(node ast.SelectNode) dialect.Result {
	var b strings.Builder
	var args []any

	switch node.Type {
	case ast.QueryCount:
		b.WriteString("SELECT COUNT(*) FROM ")
		b.WriteString(quoteIdent(node.Table))
	case ast.QueryExists:
		b.WriteString("SELECT EXISTS(SELECT 1 FROM ")
		b.WriteString(quoteIdent(node.Table))
	default:
		b.WriteString("SELECT ")
		for i, col := range node.Columns {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quoteIdent(col))
		}
		b.WriteString(" FROM ")
		b.WriteString(quoteIdent(node.Table))
	}

	if node.Where != nil {
		b.WriteString(" WHERE ")
		writeWhere(&b, &args, *node.Where)
	}

	if node.Type == ast.QueryExists {
		if node.Limit != nil {
			b.WriteString(fmt.Sprintf(" LIMIT %d", *node.Limit))
		}
		b.WriteString(")")
		return dialect.Result{SQL: b.String(), Args: args}
	}

	if len(node.OrderBy) > 0 {
		b.WriteString(" ORDER BY ")
		for i, ob := range node.OrderBy {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quoteIdent(ob.Column))
			if ob.Direction == ast.Desc {
				b.WriteString(" DESC")
			}
		}
	}

	if node.Limit != nil {
		b.WriteString(fmt.Sprintf(" LIMIT %d", *node.Limit))
	}

	if node.Offset != nil {
		b.WriteString(fmt.Sprintf(" OFFSET %d", *node.Offset))
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

// writeWhere writes a WHERE clause for SQLite.
// SQLite uses ? as native placeholder so Raw predicates pass through unchanged.
func writeWhere(b *strings.Builder, args *[]any, clause ast.WhereClause) {
	if clause.Raw != nil {
		raw := *clause.Raw
		argIdx := 0
		placeholderCount := 0
		state := 0 // 0=normal, 1=single-quote, 2=double-quote
		for i := 0; i < len(raw); i++ {
			ch := raw[i]
			switch state {
			case 0: // normal
				if ch == '\'' {
					state = 1
					b.WriteByte(ch)
				} else if ch == '"' {
					state = 2
					b.WriteByte(ch)
				} else if ch == '?' {
					placeholderCount++
					if argIdx < len(clause.RawArgs) {
						b.WriteByte('?')
						*args = append(*args, clause.RawArgs[argIdx])
						argIdx++
					} else {
						b.WriteByte(ch)
					}
				} else {
					b.WriteByte(ch)
				}
			case 1: // single-quoted string
				b.WriteByte(ch)
				if ch == '\'' {
					if i+1 < len(raw) && raw[i+1] == '\'' {
						// escaped quote ''
						i++
						b.WriteByte(raw[i])
					} else {
						state = 0
					}
				}
			case 2: // double-quoted identifier
				b.WriteByte(ch)
				if ch == '"' {
					state = 0
				}
			}
		}
		// Defense-in-depth: validate placeholder count matches args.
		if placeholderCount != len(clause.RawArgs) {
			b.Reset()
			b.WriteString(fmt.Sprintf("ERROR: raw predicate has %d placeholder(s) but %d argument(s)", placeholderCount, len(clause.RawArgs)))
		}
		return
	}

	if clause.Comparison != nil {
		writeComparison(b, args, *clause.Comparison)
		return
	}

	switch clause.LogicalOp {
	case ast.LogicalNot:
		b.WriteString("NOT (")
		writeWhere(b, args, clause.Children[0])
		b.WriteString(")")
	case ast.LogicalAnd, ast.LogicalOr:
		sep := " AND "
		if clause.LogicalOp == ast.LogicalOr {
			sep = " OR "
		}
		b.WriteString("(")
		for i, child := range clause.Children {
			if i > 0 {
				b.WriteString(sep)
			}
			writeWhere(b, args, child)
		}
		b.WriteString(")")
	}
}

func writeComparison(b *strings.Builder, args *[]any, cmp ast.ComparisonNode) {
	col := quoteIdent(cmp.Column)

	switch cmp.Op {
	case ast.OpIsNull:
		b.WriteString(col + " IS NULL")
	case ast.OpIsNotNull:
		b.WriteString(col + " IS NOT NULL")
	case ast.OpIn:
		b.WriteString(col + " IN (")
		for i, v := range cmp.Values {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("?")
			*args = append(*args, v)
		}
		b.WriteString(")")
	case ast.OpNotIn:
		b.WriteString(col + " NOT IN (")
		for i, v := range cmp.Values {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString("?")
			*args = append(*args, v)
		}
		b.WriteString(")")
	case ast.OpBetween:
		b.WriteString(fmt.Sprintf("%s BETWEEN ? AND ?", col))
		*args = append(*args, cmp.Values[0], cmp.Values[1])
	default:
		op := operatorToSQL(cmp.Op)
		b.WriteString(fmt.Sprintf("%s %s ?", col, op))
		*args = append(*args, cmp.Value)
	}
}

// operatorToSQL maps AST operators to SQLite SQL operator strings.
// ILIKE maps to LIKE because SQLite LIKE is case-insensitive for ASCII by default.
func operatorToSQL(op ast.Operator) string {
	switch op {
	case ast.OpEq:
		return "="
	case ast.OpNEQ:
		return "!="
	case ast.OpGT:
		return ">"
	case ast.OpGTE:
		return ">="
	case ast.OpLT:
		return "<"
	case ast.OpLTE:
		return "<="
	case ast.OpLike:
		return "LIKE"
	case ast.OpILike:
		// SQLite LIKE is case-insensitive for ASCII; no ILIKE keyword.
		return "LIKE"
	default:
		return "="
	}
}

func (s *SQLite) BuildInsert(table string, columns []string, values []any, _ []string) dialect.Result {
	var b strings.Builder
	b.WriteString("INSERT INTO ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" (")
	for i, col := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(col))
	}
	b.WriteString(") VALUES (")
	for i := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString("?")
	}
	b.WriteString(")")
	// SQLite does not support RETURNING; returningCols is ignored.
	return dialect.Result{SQL: b.String(), Args: values}
}

func (s *SQLite) BuildUpdate(table string, changes []dialect.ColumnValue, pkColumn string, pkValue any) dialect.Result {
	var b strings.Builder
	var args []any
	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	for i, cv := range changes {
		if i > 0 {
			b.WriteString(", ")
		}
		if raw, ok := cv.Value.(dialect.RawExpr); ok {
			b.WriteString(fmt.Sprintf("%s = %s", quoteIdent(cv.Column), raw.SQL))
		} else {
			b.WriteString(fmt.Sprintf("%s = ?", quoteIdent(cv.Column)))
			args = append(args, cv.Value)
		}
	}
	b.WriteString(fmt.Sprintf(" WHERE %s = ?", quoteIdent(pkColumn)))
	args = append(args, pkValue)
	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildDelete(table string, pkColumn string, pkValue any) dialect.Result {
	sql := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoteIdent(table), quoteIdent(pkColumn))
	return dialect.Result{SQL: sql, Args: []any{pkValue}}
}

func (s *SQLite) BuildSoftDelete(table string, pkColumn string, pkValue any) dialect.Result {
	sql := fmt.Sprintf(
		"UPDATE %s SET %s = CURRENT_TIMESTAMP WHERE %s = ?",
		quoteIdent(table), quoteIdent("deleted_at"), quoteIdent(pkColumn),
	)
	return dialect.Result{SQL: sql, Args: []any{pkValue}}
}

// BuildUpdateVersioned generates a versioned UPDATE for SQLite.
// SQLite does not support RETURNING, so the new version cannot be retrieved
// from the statement itself; the caller must increment the version client-side.
func (s *SQLite) BuildUpdateVersioned(table string, changes []dialect.ColumnValue, pkColumn string, pkValue any, versionCol string, currentVersion int) dialect.Result {
	var b strings.Builder
	var args []any

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")

	for i, cv := range changes {
		if i > 0 {
			b.WriteString(", ")
		}
		if raw, ok := cv.Value.(dialect.RawExpr); ok {
			b.WriteString(fmt.Sprintf("%s = %s", quoteIdent(cv.Column), raw.SQL))
		} else {
			b.WriteString(fmt.Sprintf("%s = ?", quoteIdent(cv.Column)))
			args = append(args, cv.Value)
		}
	}

	b.WriteString(fmt.Sprintf(", %s = %s + 1", quoteIdent(versionCol), quoteIdent(versionCol)))

	b.WriteString(fmt.Sprintf(" WHERE %s = ?", quoteIdent(pkColumn)))
	args = append(args, pkValue)

	b.WriteString(fmt.Sprintf(" AND %s = ?", quoteIdent(versionCol)))
	args = append(args, currentVersion)

	// No RETURNING clause — SQLite does not support it.
	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildBulkInsert(table string, columns []string, rows [][]any) dialect.Result {
	var b strings.Builder
	var args []any

	b.WriteString("INSERT INTO ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" (")
	for i, col := range columns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(col))
	}
	b.WriteString(") VALUES ")

	for ri, row := range rows {
		if ri > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for ci, val := range row {
			if ci > 0 {
				b.WriteString(", ")
			}
			b.WriteString("?")
			args = append(args, val)
		}
		b.WriteString(")")
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildBulkUpdate(table string, sets []dialect.ColumnValue, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	for i, cv := range sets {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s = ?", quoteIdent(cv.Column)))
		args = append(args, cv.Value)
	}

	if where != nil {
		b.WriteString(" WHERE ")
		writeWhere(&b, &args, *where)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildBulkDelete(table string, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any

	b.WriteString("DELETE FROM ")
	b.WriteString(quoteIdent(table))

	if where != nil {
		b.WriteString(" WHERE ")
		writeWhere(&b, &args, *where)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildBulkSoftDelete(table string, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	b.WriteString(quoteIdent("deleted_at"))
	b.WriteString(" = CURRENT_TIMESTAMP WHERE ")
	b.WriteString(quoteIdent("deleted_at"))
	b.WriteString(" IS NULL")

	if where != nil {
		b.WriteString(" AND ")
		writeWhere(&b, &args, *where)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (s *SQLite) BuildBulkUpsert(table string, columns []string, rows [][]any, conflictCols []string, updateCols []string) dialect.Result {
	result := s.BuildBulkInsert(table, columns, rows)

	var b strings.Builder
	b.WriteString(result.SQL)
	b.WriteString(" ON CONFLICT (")
	for i, col := range conflictCols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(quoteIdent(col))
	}
	b.WriteString(") DO UPDATE SET ")
	for i, col := range updateCols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s = EXCLUDED.%s", quoteIdent(col), quoteIdent(col)))
	}

	return dialect.Result{SQL: b.String(), Args: result.Args}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
