package dialect

import "github.com/alternayte/drel/internal/ast"

type Result struct {
	SQL  string
	Args []any
}

type Dialect interface {
	BuildSelect(node ast.SelectNode) Result
}
