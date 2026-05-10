package drel

type Relation[T any] struct {
	name string
}

func NewRelation[T any](name string) Relation[T] {
	return Relation[T]{name: name}
}

func (r Relation[T]) Name() string { return r.name }
