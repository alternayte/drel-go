package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/ast"
)

// RelationType describes the kind of relationship between two models.
type RelationType int

const (
	// HasMany indicates the parent has many related entities (1:N).
	HasMany RelationType = iota
	// HasOne indicates the parent has one related entity (1:1).
	HasOne
	// BelongsTo indicates the entity references a parent via a foreign key.
	BelongsTo
)

// RelationInfo describes a relationship between two models.
type RelationInfo struct {
	Name        string
	Type        RelationType
	FKColumn    string
	RelatedMeta *ModelMetaBase
	FieldSetter func(parent any, related any)
}

// IncludeSpec wraps a RelationInfo for use with Include queries.
type IncludeSpec struct {
	rel *RelationInfo
}

// NewIncludeSpec creates an IncludeSpec from a RelationInfo.
func NewIncludeSpec(rel *RelationInfo) IncludeSpec {
	return IncludeSpec{rel: rel}
}

// IncludableQuery constructs queries that eagerly load related entities.
type IncludableQuery[T any] struct {
	repo     *Repository[T]
	includes []IncludeSpec
}

// Include begins a query that will eagerly load the given relationships.
func (r *Repository[T]) Include(rels ...IncludeSpec) *IncludableQuery[T] {
	return &IncludableQuery[T]{
		repo:     r,
		includes: rels,
	}
}

// Find looks up a single record by primary key and loads included relationships.
func (q *IncludableQuery[T]) Find(ctx context.Context, id any) (*T, error) {
	entity, err := q.repo.Find(ctx, id)
	if err != nil {
		return nil, err
	}

	parents := []any{entity}
	parentMeta := ToMetaBase(&q.repo.meta)
	exec := &includeExecutor{
		engine:     q.repo.engine,
		parentMeta: parentMeta,
	}
	if err := exec.loadRelations(ctx, parents, q.includes); err != nil {
		return nil, err
	}
	return entity, nil
}

// All returns all records and loads included relationships.
func (q *IncludableQuery[T]) All(ctx context.Context) ([]*T, error) {
	entities, err := q.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return entities, nil
	}

	parents := make([]any, len(entities))
	for i, e := range entities {
		parents[i] = e
	}
	parentMeta := ToMetaBase(&q.repo.meta)
	exec := &includeExecutor{
		engine:     q.repo.engine,
		parentMeta: parentMeta,
	}
	if err := exec.loadRelations(ctx, parents, q.includes); err != nil {
		return nil, err
	}
	return entities, nil
}

// includeExecutor runs split queries to load related entities.
type includeExecutor struct {
	engine     *Engine
	parentMeta *ModelMetaBase
}

func (ie *includeExecutor) loadRelations(ctx context.Context, parents []any, includes []IncludeSpec) error {
	for _, inc := range includes {
		if err := ie.loadRelation(ctx, parents, inc.rel); err != nil {
			return fmt.Errorf("drel: include %s: %w", inc.rel.Name, err)
		}
	}
	return nil
}

func (ie *includeExecutor) loadRelation(ctx context.Context, parents []any, rel *RelationInfo) error {
	switch rel.Type {
	case HasMany, HasOne:
		return ie.loadHasManyOrOne(ctx, parents, rel)
	case BelongsTo:
		return ie.loadBelongsTo(ctx, parents, rel)
	default:
		return fmt.Errorf("unknown relation type %d", rel.Type)
	}
}

// loadHasManyOrOne executes: SELECT * FROM related_table WHERE fk_column IN (pk1, pk2, ...)
// then groups results by the FK value and sets them on each parent.
func (ie *includeExecutor) loadHasManyOrOne(ctx context.Context, parents []any, rel *RelationInfo) error {
	// Collect parent PK values.
	pkValues := make([]any, len(parents))
	for i, p := range parents {
		pkValues[i] = ie.parentMeta.PKValue(p)
	}

	// Query related entities.
	related, err := ie.queryByColumn(ctx, rel.RelatedMeta, rel.FKColumn, pkValues)
	if err != nil {
		return err
	}

	// Find the FK column index in the related meta.
	fkIdx := findColumnIndex(rel.RelatedMeta.Columns, rel.FKColumn)
	if fkIdx < 0 {
		return fmt.Errorf("FK column %q not found in %s columns", rel.FKColumn, rel.RelatedMeta.Table)
	}

	if rel.Type == HasMany {
		// Group related entities by FK value.
		grouped := make(map[any][]any)
		for _, r := range related {
			fkVal := rel.RelatedMeta.ColumnValue(r, fkIdx)
			grouped[fkVal] = append(grouped[fkVal], r)
		}
		// Set on each parent.
		for _, p := range parents {
			pk := ie.parentMeta.PKValue(p)
			items := grouped[pk]
			if items == nil {
				items = []any{}
			}
			rel.FieldSetter(p, items)
		}
	} else {
		// HasOne: index by FK value.
		byFK := make(map[any]any)
		for _, r := range related {
			fkVal := rel.RelatedMeta.ColumnValue(r, fkIdx)
			byFK[fkVal] = r
		}
		for _, p := range parents {
			pk := ie.parentMeta.PKValue(p)
			if r, ok := byFK[pk]; ok {
				rel.FieldSetter(p, r)
			}
		}
	}

	return nil
}

// loadBelongsTo executes: SELECT * FROM related_table WHERE pk IN (fk1, fk2, ...)
// Collects FK values from parents, then matches related entities by PK.
func (ie *includeExecutor) loadBelongsTo(ctx context.Context, parents []any, rel *RelationInfo) error {
	// Find the FK column index in the parent meta.
	fkIdx := findColumnIndex(ie.parentMeta.Columns, rel.FKColumn)
	if fkIdx < 0 {
		return fmt.Errorf("FK column %q not found in %s columns", rel.FKColumn, ie.parentMeta.Table)
	}

	// Collect FK values from parents (deduplicating).
	seen := make(map[any]bool)
	var fkValues []any
	for _, p := range parents {
		fk := ie.parentMeta.ColumnValue(p, fkIdx)
		if fk != nil && !seen[fk] {
			seen[fk] = true
			fkValues = append(fkValues, fk)
		}
	}

	if len(fkValues) == 0 {
		return nil
	}

	// Query related entities by their PK.
	related, err := ie.queryByColumn(ctx, rel.RelatedMeta, rel.RelatedMeta.PKColumn, fkValues)
	if err != nil {
		return err
	}

	// Index related by PK.
	byPK := make(map[any]any)
	for _, r := range related {
		pk := rel.RelatedMeta.PKValue(r)
		byPK[pk] = r
	}

	// Set on each parent.
	for _, p := range parents {
		fk := ie.parentMeta.ColumnValue(p, fkIdx)
		if r, ok := byPK[fk]; ok {
			rel.FieldSetter(p, r)
		}
	}

	return nil
}

// queryByColumn executes SELECT * FROM table WHERE column IN (values...)
func (ie *includeExecutor) queryByColumn(ctx context.Context, meta *ModelMetaBase, column string, values []any) ([]any, error) {
	node := ast.SelectNode{
		Table:   meta.Table,
		Columns: meta.Columns,
		Where: &ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpIn,
				Values: values,
			},
		},
		Type: ast.QuerySelect,
	}

	result := ie.engine.Dialect().BuildSelect(node)
	rows, err := ie.engine.Driver().Query(ctx, result.SQL, result.Args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []any
	for rows.Next() {
		item, err := meta.ScanRow(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// findColumnIndex returns the index of the named column, or -1 if not found.
func findColumnIndex(columns []string, name string) int {
	for i, c := range columns {
		if c == name {
			return i
		}
	}
	return -1
}
