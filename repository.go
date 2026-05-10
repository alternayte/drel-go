package drel

import (
	"context"

	"github.com/alternayte/drel/internal/ast"
)

// ModelMeta describes the database mapping for a model type T.
type ModelMeta[T any] struct {
	Table          string
	Columns        []string
	PKColumn       string
	Scan           func(Row) (*T, error)
	Snapshot       func(*T) any
	Diff           func(*T, any) []FieldChange
	PKValue        func(*T) any
	InsertColumns  func(*T) ([]string, []any)
	ScanReturning  func(*T, Row) error
}

func toMetaBase[T any](meta *ModelMeta[T]) *modelMetaBase {
	base := &modelMetaBase{
		Table:    meta.Table,
		Columns:  meta.Columns,
		PKColumn: meta.PKColumn,
		Snapshot: func(entity any) any {
			return meta.Snapshot(entity.(*T))
		},
		Diff: func(entity any, snapshot any) []FieldChange {
			return meta.Diff(entity.(*T), snapshot)
		},
		PKValue: func(entity any) any {
			return meta.PKValue(entity.(*T))
		},
		InsertColumns: func(entity any) ([]string, []any) {
			return meta.InsertColumns(entity.(*T))
		},
		ScanRow: func(row Row) (any, error) {
			return meta.Scan(row)
		},
	}
	if meta.ScanReturning != nil {
		base.ScanReturning = func(entity any, row Row) error {
			return meta.ScanReturning(entity.(*T), row)
		}
	}
	return base
}

// Repository provides typed query access for a specific model.
type Repository[T any] struct {
	engine *Engine
	meta   ModelMeta[T]
}

// NewRepository creates a new Repository for the given model metadata.
func NewRepository[T any](engine *Engine, meta ModelMeta[T]) *Repository[T] {
	return &Repository[T]{
		engine: engine,
		meta:   meta,
	}
}

func (r *Repository[T]) newBuilder() *QueryBuilder[T] {
	return newQueryBuilder(r.engine, &r.meta)
}

// Find looks up a single record by its primary key.
func (r *Repository[T]) Find(ctx context.Context, id any) (*T, error) {
	return r.newBuilder().
		Where(newComparison(r.meta.PKColumn, ast.OpEq, id)).
		First(ctx)
}

// Where starts a filtered query.
func (r *Repository[T]) Where(pred Predicate) *QueryBuilder[T] {
	return r.newBuilder().Where(pred)
}

// OrderBy starts an ordered query.
func (r *Repository[T]) OrderBy(exprs ...OrderExpr) *QueryBuilder[T] {
	return r.newBuilder().OrderBy(exprs...)
}

// Limit starts a query with a row limit.
func (r *Repository[T]) Limit(n int) *QueryBuilder[T] {
	return r.newBuilder().Limit(n)
}

// All returns all records for this model.
func (r *Repository[T]) All(ctx context.Context) ([]*T, error) {
	return r.newBuilder().All(ctx)
}

// First returns the first record or ErrNotFound.
func (r *Repository[T]) First(ctx context.Context) (*T, error) {
	return r.newBuilder().First(ctx)
}

// FirstOrNil returns the first record or nil.
func (r *Repository[T]) FirstOrNil(ctx context.Context) (*T, error) {
	return r.newBuilder().FirstOrNil(ctx)
}

// Count returns the total number of records for this model.
func (r *Repository[T]) Count(ctx context.Context) (int, error) {
	return r.newBuilder().Count(ctx)
}

// Exists returns true if any records exist for this model.
func (r *Repository[T]) Exists(ctx context.Context) (bool, error) {
	return r.newBuilder().Exists(ctx)
}
