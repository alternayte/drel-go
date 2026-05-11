package drel

// FieldChange represents a single column value that has changed on an entity.
type FieldChange struct {
	Column string
	Value  any
}

// RawExpr represents a raw SQL expression to embed in a query (e.g., NOW()).
type RawExpr struct {
	SQL string
}
