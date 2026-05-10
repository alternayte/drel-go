package drel

import "github.com/alternayte/drel/internal/ast"

type NamedFilter struct {
	Name   string
	Clause ast.WhereClause
}
