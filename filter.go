package drel

import "github.com/alternayte/drel/internal/ast"

type NamedFilter struct {
	Name   string
	Clause ast.WhereClause
}

var SoftDeleteFilter = NamedFilter{
	Name: "soft_delete",
	Clause: ast.WhereClause{
		Comparison: &ast.ComparisonNode{Column: "deleted_at", Op: ast.OpIsNull},
	},
}
