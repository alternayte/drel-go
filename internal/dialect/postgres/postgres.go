package postgres

import (
	"fmt"
	"strings"

	"github.com/alternayte/drel/internal/ast"
	"github.com/alternayte/drel/internal/dialect"
)

type Postgres struct{}

func New() *Postgres { return &Postgres{} }

func (p *Postgres) SupportsReturning() bool { return true }

func (p *Postgres) Now() string { return "NOW()" }

func (p *Postgres) Explain(query string) (string, bool) { return "EXPLAIN " + query, true }

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
		for aggIdx, agg := range node.Aggregates {
			if len(node.Columns) > 0 || aggIdx > 0 {
				b.WriteString(", ")
			}
			b.WriteString(aggFuncSQL(agg.Func))
			b.WriteString("(")
			b.WriteString(quoteIdent(agg.Column))
			b.WriteString(")")
			if agg.Alias != "" {
				b.WriteString(" AS ")
				b.WriteString(quoteIdent(agg.Alias))
			}
		}
		b.WriteString(" FROM ")
		b.WriteString(quoteIdent(node.Table))
	}

	if node.Where != nil {
		b.WriteString(" WHERE ")
		paramIdx = writeWhere(&b, &args, *node.Where, paramIdx)
	}

	if node.Type != ast.QueryExists && node.Type != ast.QueryCount {
		if len(node.GroupBy) > 0 {
			b.WriteString(" GROUP BY ")
			for i, col := range node.GroupBy {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(quoteIdent(col))
			}
		}
		if node.Having != nil {
			b.WriteString(" HAVING ")
			paramIdx = writeWhere(&b, &args, *node.Having, paramIdx)
		}
	}

	if node.Type == ast.QueryExists {
		if node.Limit != nil {
			b.WriteString(fmt.Sprintf(" LIMIT %d", *node.Limit))
		}
		b.WriteString(")")
		return dialect.Result{SQL: b.String(), Args: args}
	}

	// COUNT carries only WHERE — ordering/limit/offset would corrupt the single-row result.
	if node.Type == ast.QueryCount {
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
			switch ob.Nulls {
			case ast.NullsFirst:
				b.WriteString(" NULLS FIRST")
			case ast.NullsLast:
				b.WriteString(" NULLS LAST")
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
	if clause.Raw != nil {
		raw := *clause.Raw
		argIdx := 0
		placeholderCount := 0
		state := 0 // 0=normal, 1=single-quote, 2=double-quote, 3=dollar-quote
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
				} else if ch == '$' && i+1 < len(raw) && raw[i+1] == '$' {
					state = 3
					b.WriteByte(ch)
					i++
					b.WriteByte(raw[i])
				} else if ch == '?' {
					placeholderCount++
					if argIdx < len(clause.RawArgs) {
						b.WriteString(fmt.Sprintf("$%d", paramIdx))
						*args = append(*args, clause.RawArgs[argIdx])
						argIdx++
						paramIdx++
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
			case 3: // dollar-quoted string
				b.WriteByte(ch)
				if ch == '$' && i+1 < len(raw) && raw[i+1] == '$' {
					i++
					b.WriteByte(raw[i])
					state = 0
				}
			}
		}
		// Defense-in-depth: validate placeholder count matches args.
		if placeholderCount != len(clause.RawArgs) {
			b.Reset()
			b.WriteString(fmt.Sprintf("ERROR: raw predicate has %d placeholder(s) but %d argument(s)", placeholderCount, len(clause.RawArgs)))
		}
		return paramIdx
	}

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
	case ast.OpNotIn:
		b.WriteString(col + " NOT IN (")
		for i, v := range cmp.Values {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(fmt.Sprintf("$%d", paramIdx))
			*args = append(*args, v)
			paramIdx++
		}
		b.WriteString(")")
	case ast.OpBetween:
		b.WriteString(fmt.Sprintf("%s BETWEEN $%d AND $%d", col, paramIdx, paramIdx+1))
		*args = append(*args, cmp.Values[0], cmp.Values[1])
		paramIdx += 2
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
	case ast.OpILike:
		return "ILIKE"
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
		if raw, ok := cv.Value.(dialect.RawExpr); ok {
			b.WriteString(fmt.Sprintf("%s = %s", quoteIdent(cv.Column), raw.SQL))
		} else {
			b.WriteString(fmt.Sprintf("%s = $%d", quoteIdent(cv.Column), paramIdx))
			args = append(args, cv.Value)
			paramIdx++
		}
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
		if raw, ok := cv.Value.(dialect.RawExpr); ok {
			b.WriteString(fmt.Sprintf("%s = %s", quoteIdent(cv.Column), raw.SQL))
		} else {
			b.WriteString(fmt.Sprintf("%s = $%d", quoteIdent(cv.Column), paramIdx))
			args = append(args, cv.Value)
			paramIdx++
		}
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

func aggFuncSQL(f ast.AggFunc) string {
	switch f {
	case ast.AggSum:
		return "SUM"
	case ast.AggAvg:
		return "AVG"
	case ast.AggMin:
		return "MIN"
	case ast.AggMax:
		return "MAX"
	case ast.AggCount:
		return "COUNT"
	default:
		return "COUNT"
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
