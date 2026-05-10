package postgres

import (
	"fmt"
	"strings"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
)

type Postgres struct{}

func New() *Postgres { return &Postgres{} }

func (p *Postgres) BuildSelect(node ast.SelectNode) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

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
		paramIdx = writeWhere(&b, &args, *node.Where, paramIdx)
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

func writeWhere(b *strings.Builder, args *[]any, clause ast.WhereClause, paramIdx int) int {
	if clause.Comparison != nil {
		return writeComparison(b, args, *clause.Comparison, paramIdx)
	}

	switch clause.LogicalOp {
	case ast.LogicalNot:
		b.WriteString("NOT (")
		paramIdx = writeWhere(b, args, clause.Children[0], paramIdx)
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
			paramIdx = writeWhere(b, args, child, paramIdx)
		}
		b.WriteString(")")
	}

	return paramIdx
}

func writeComparison(b *strings.Builder, args *[]any, cmp ast.ComparisonNode, paramIdx int) int {
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
			b.WriteString(fmt.Sprintf("$%d", paramIdx))
			*args = append(*args, v)
			paramIdx++
		}
		b.WriteString(")")
	default:
		op := operatorToSQL(cmp.Op)
		b.WriteString(fmt.Sprintf("%s %s $%d", col, op, paramIdx))
		*args = append(*args, cmp.Value)
		paramIdx++
	}

	return paramIdx
}

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
	default:
		return "="
	}
}

func (p *Postgres) BuildInsert(table string, columns []string, values []any, returningCols []string) dialect.Result {
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
		b.WriteString(fmt.Sprintf("$%d", i+1))
	}
	b.WriteString(")")
	if len(returningCols) > 0 {
		b.WriteString(" RETURNING ")
		for i, col := range returningCols {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(quoteIdent(col))
		}
	}
	return dialect.Result{SQL: b.String(), Args: values}
}

func (p *Postgres) BuildUpdate(table string, changes []dialect.ColumnValue, pkColumn string, pkValue any) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1
	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	for i, cv := range changes {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s = $%d", quoteIdent(cv.Column), paramIdx))
		args = append(args, cv.Value)
		paramIdx++
	}
	b.WriteString(fmt.Sprintf(" WHERE %s = $%d", quoteIdent(pkColumn), paramIdx))
	args = append(args, pkValue)
	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildDelete(table string, pkColumn string, pkValue any) dialect.Result {
	sql := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", quoteIdent(table), quoteIdent(pkColumn))
	return dialect.Result{SQL: sql, Args: []any{pkValue}}
}

func (p *Postgres) BuildSoftDelete(table string, pkColumn string, pkValue any) dialect.Result {
	sql := fmt.Sprintf(
		"UPDATE %s SET %s = NOW() WHERE %s = $1",
		quoteIdent(table), quoteIdent("deleted_at"), quoteIdent(pkColumn),
	)
	return dialect.Result{SQL: sql, Args: []any{pkValue}}
}

func (p *Postgres) BuildUpdateVersioned(table string, changes []dialect.ColumnValue, pkColumn string, pkValue any, versionCol string, currentVersion int) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")

	for i, cv := range changes {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s = $%d", quoteIdent(cv.Column), paramIdx))
		args = append(args, cv.Value)
		paramIdx++
	}

	b.WriteString(fmt.Sprintf(", %s = %s + 1", quoteIdent(versionCol), quoteIdent(versionCol)))

	b.WriteString(fmt.Sprintf(" WHERE %s = $%d", quoteIdent(pkColumn), paramIdx))
	args = append(args, pkValue)
	paramIdx++

	b.WriteString(fmt.Sprintf(" AND %s = $%d", quoteIdent(versionCol), paramIdx))
	args = append(args, currentVersion)

	b.WriteString(fmt.Sprintf(" RETURNING %s", quoteIdent(versionCol)))

	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildBulkInsert(table string, columns []string, rows [][]any) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

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
			b.WriteString(fmt.Sprintf("$%d", paramIdx))
			args = append(args, val)
			paramIdx++
		}
		b.WriteString(")")
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildBulkUpdate(table string, sets []dialect.ColumnValue, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	for i, cv := range sets {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s = $%d", quoteIdent(cv.Column), paramIdx))
		args = append(args, cv.Value)
		paramIdx++
	}

	if where != nil {
		b.WriteString(" WHERE ")
		writeWhere(&b, &args, *where, paramIdx)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildBulkDelete(table string, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

	b.WriteString("DELETE FROM ")
	b.WriteString(quoteIdent(table))

	if where != nil {
		b.WriteString(" WHERE ")
		writeWhere(&b, &args, *where, paramIdx)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildBulkSoftDelete(table string, where *ast.WhereClause) dialect.Result {
	var b strings.Builder
	var args []any
	paramIdx := 1

	b.WriteString("UPDATE ")
	b.WriteString(quoteIdent(table))
	b.WriteString(" SET ")
	b.WriteString(quoteIdent("deleted_at"))
	b.WriteString(" = NOW() WHERE ")
	b.WriteString(quoteIdent("deleted_at"))
	b.WriteString(" IS NULL")

	if where != nil {
		b.WriteString(" AND ")
		writeWhere(&b, &args, *where, paramIdx)
	}

	return dialect.Result{SQL: b.String(), Args: args}
}

func (p *Postgres) BuildBulkUpsert(table string, columns []string, rows [][]any, conflictCols []string, updateCols []string) dialect.Result {
	result := p.BuildBulkInsert(table, columns, rows)

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
	return `"` + name + `"`
}
