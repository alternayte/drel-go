package drel

import (
	"cmp"

	"github.com/alternayte/drel/internal/ast"
)

type Column[T any] struct {
	name string
}

func NewCol[T any](name string) Column[T] {
	return Column[T]{name: name}
}

func (c Column[T]) Name() string { return c.name }

func (c Column[T]) Eq(v T) Predicate {
	return newComparison(c.name, ast.OpEq, v)
}

func (c Column[T]) NEQ(v T) Predicate {
	return newComparison(c.name, ast.OpNEQ, v)
}

func (c Column[T]) IsNull() Predicate {
	return newNullCheck(c.name, true)
}

func (c Column[T]) IsNotNull() Predicate {
	return newNullCheck(c.name, false)
}

func (c Column[T]) In(vs ...T) Predicate {
	vals := make([]any, len(vs))
	for i, v := range vs {
		vals[i] = v
	}
	return newInComparison(c.name, vals)
}

func (c Column[T]) Asc() OrderExpr {
	return OrderExpr{column: c.name, direction: ast.Asc}
}

func (c Column[T]) Desc() OrderExpr {
	return OrderExpr{column: c.name, direction: ast.Desc}
}

type OrderedColumn[T cmp.Ordered] struct {
	col Column[T]
}

func NewOrderedCol[T cmp.Ordered](name string) OrderedColumn[T] {
	return OrderedColumn[T]{col: NewCol[T](name)}
}

func (c OrderedColumn[T]) Name() string        { return c.col.name }
func (c OrderedColumn[T]) Eq(v T) Predicate    { return c.col.Eq(v) }
func (c OrderedColumn[T]) NEQ(v T) Predicate   { return c.col.NEQ(v) }
func (c OrderedColumn[T]) IsNull() Predicate   { return c.col.IsNull() }
func (c OrderedColumn[T]) IsNotNull() Predicate { return c.col.IsNotNull() }
func (c OrderedColumn[T]) In(vs ...T) Predicate { return c.col.In(vs...) }
func (c OrderedColumn[T]) Asc() OrderExpr      { return c.col.Asc() }
func (c OrderedColumn[T]) Desc() OrderExpr     { return c.col.Desc() }

func (c OrderedColumn[T]) GT(v T) Predicate {
	return newComparison(c.col.name, ast.OpGT, v)
}

func (c OrderedColumn[T]) GTE(v T) Predicate {
	return newComparison(c.col.name, ast.OpGTE, v)
}

func (c OrderedColumn[T]) LT(v T) Predicate {
	return newComparison(c.col.name, ast.OpLT, v)
}

func (c OrderedColumn[T]) LTE(v T) Predicate {
	return newComparison(c.col.name, ast.OpLTE, v)
}

type StringColumn struct {
	col Column[string]
}

func NewStringCol(name string) StringColumn {
	return StringColumn{col: NewCol[string](name)}
}

func (c StringColumn) Name() string             { return c.col.name }
func (c StringColumn) Eq(v string) Predicate    { return c.col.Eq(v) }
func (c StringColumn) NEQ(v string) Predicate   { return c.col.NEQ(v) }
func (c StringColumn) IsNull() Predicate        { return c.col.IsNull() }
func (c StringColumn) IsNotNull() Predicate     { return c.col.IsNotNull() }
func (c StringColumn) In(vs ...string) Predicate { return c.col.In(vs...) }
func (c StringColumn) Asc() OrderExpr           { return c.col.Asc() }
func (c StringColumn) Desc() OrderExpr          { return c.col.Desc() }

func (c StringColumn) Like(pattern string) Predicate {
	return newComparison(c.col.name, ast.OpLike, pattern)
}

func (c StringColumn) Contains(substr string) Predicate {
	return newComparison(c.col.name, ast.OpLike, "%"+substr+"%")
}

func (c StringColumn) HasPrefix(prefix string) Predicate {
	return newComparison(c.col.name, ast.OpLike, prefix+"%")
}

func (c StringColumn) HasSuffix(suffix string) Predicate {
	return newComparison(c.col.name, ast.OpLike, "%"+suffix)
}

func (c StringColumn) GT(v string) Predicate  { return newComparison(c.col.name, ast.OpGT, v) }
func (c StringColumn) GTE(v string) Predicate { return newComparison(c.col.name, ast.OpGTE, v) }
func (c StringColumn) LT(v string) Predicate  { return newComparison(c.col.name, ast.OpLT, v) }
func (c StringColumn) LTE(v string) Predicate { return newComparison(c.col.name, ast.OpLTE, v) }

type BoolColumn struct {
	col Column[bool]
}

func NewBoolCol(name string) BoolColumn {
	return BoolColumn{col: NewCol[bool](name)}
}

func (c BoolColumn) Name() string         { return c.col.name }
func (c BoolColumn) Eq(v bool) Predicate  { return c.col.Eq(v) }
func (c BoolColumn) NEQ(v bool) Predicate { return c.col.NEQ(v) }
func (c BoolColumn) IsNull() Predicate    { return c.col.IsNull() }
func (c BoolColumn) IsNotNull() Predicate { return c.col.IsNotNull() }
func (c BoolColumn) Asc() OrderExpr       { return c.col.Asc() }
func (c BoolColumn) Desc() OrderExpr      { return c.col.Desc() }

func (c BoolColumn) IsTrue() Predicate  { return c.col.Eq(true) }
func (c BoolColumn) IsFalse() Predicate { return c.col.Eq(false) }
