package dialect

import "github.com/alternayte/drel/internal/ast"

// Result holds a generated SQL statement and its bound arguments.
type Result struct {
	SQL  string
	Args []any
}

// ColumnValue pairs a column name with its new value for mutation queries.
type ColumnValue struct {
	Column string
	Value  any
}

// Dialect generates SQL for a specific database backend.
type Dialect interface {
	BuildSelect(node ast.SelectNode) Result
	BuildInsert(table string, columns []string, values []any, returningCols []string) Result
	BuildUpdate(table string, changes []ColumnValue, pkColumn string, pkValue any) Result
	BuildDelete(table string, pkColumn string, pkValue any) Result
	BuildSoftDelete(table string, pkColumn string, pkValue any) Result
	BuildUpdateVersioned(table string, changes []ColumnValue, pkColumn string, pkValue any, versionCol string, currentVersion int) Result
	BuildBulkInsert(table string, columns []string, rows [][]any) Result
	BuildBulkUpdate(table string, sets []ColumnValue, where *ast.WhereClause) Result
	BuildBulkDelete(table string, where *ast.WhereClause) Result
	BuildBulkSoftDelete(table string, where *ast.WhereClause) Result
	BuildBulkUpsert(table string, columns []string, rows [][]any, conflictCols []string, updateCols []string) Result
}
