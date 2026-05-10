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
}
