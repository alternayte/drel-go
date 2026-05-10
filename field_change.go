package drel

// FieldChange represents a single column value that has changed on an entity.
type FieldChange struct {
	Column string
	Value  any
}
