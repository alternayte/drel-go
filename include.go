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
	// ManyToMany indicates a many-to-many relationship via a pivot table.
	ManyToMany
)

// RelationInfo describes a relationship between two models.
type RelationInfo struct {
	Name        string
	Type        RelationType
	FKColumn    string
	JoinTable   string // pivot table name (many-to-many only)
	RefColumn   string // target FK column in pivot table (many-to-many only)
	RelatedMeta *ModelMetaBase
	FieldSetter func(parent any, related any)
}

// IncludeSpec wraps a RelationInfo for use with Include queries.
type IncludeSpec struct {
	rel      *RelationInfo
	unscoped bool
	wheres   []ast.WhereClause
	orderBy  []ast.OrderByExpr
	limit    *int
}

// NewIncludeSpec creates an IncludeSpec from a RelationInfo.
func NewIncludeSpec(rel *RelationInfo) IncludeSpec {
	return IncludeSpec{rel: rel}
}

// Unscoped returns a copy of the IncludeSpec that bypasses global filters
// (e.g. soft delete) when loading the related entities.
func (s IncludeSpec) Unscoped() IncludeSpec {
	c := s
	c.unscoped = true
	return c
}

// Where returns a copy of the IncludeSpec with an additional filter predicate
// applied to the related entities query.
func (s IncludeSpec) Where(pred Predicate) IncludeSpec {
	c := s
	c.wheres = append(append([]ast.WhereClause(nil), s.wheres...), pred.clause)
	return c
}

// OrderBy returns a copy of the IncludeSpec with the given ordering applied
// to the related entities query.
func (s IncludeSpec) OrderBy(exprs ...OrderExpr) IncludeSpec {
	c := s
	newOrder := make([]ast.OrderByExpr, len(s.orderBy))
	copy(newOrder, s.orderBy)
	for _, e := range exprs {
		newOrder = append(newOrder, e.ToAST())
	}
	c.orderBy = newOrder
	return c
}

// Limit returns a copy of the IncludeSpec that limits the number of related
// entities loaded per parent batch.
func (s IncludeSpec) Limit(n int) IncludeSpec {
	c := s
	c.limit = &n
	return c
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
		if err := ie.loadRelation(ctx, parents, inc); err != nil {
			return fmt.Errorf("drel: include %s: %w", inc.rel.Name, err)
		}
	}
	return nil
}

func (ie *includeExecutor) loadRelation(ctx context.Context, parents []any, inc IncludeSpec) error {
	switch inc.rel.Type {
	case HasMany, HasOne:
		return ie.loadHasManyOrOne(ctx, parents, inc)
	case BelongsTo:
		return ie.loadBelongsTo(ctx, parents, inc)
	case ManyToMany:
		return ie.loadManyToMany(ctx, parents, inc)
	default:
		return fmt.Errorf("unknown relation type %d", inc.rel.Type)
	}
}

// loadHasManyOrOne executes: SELECT * FROM related_table WHERE fk_column IN (pk1, pk2, ...)
// then groups results by the FK value and sets them on each parent.
func (ie *includeExecutor) loadHasManyOrOne(ctx context.Context, parents []any, inc IncludeSpec) error {
	rel := inc.rel
	// Collect parent PK values.
	pkValues := make([]any, len(parents))
	for i, p := range parents {
		pkValues[i] = ie.parentMeta.PKValue(p)
	}

	// Query related entities.
	related, err := ie.queryByColumn(ctx, rel.RelatedMeta, rel.FKColumn, pkValues, inc)
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
func (ie *includeExecutor) loadBelongsTo(ctx context.Context, parents []any, inc IncludeSpec) error {
	rel := inc.rel
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
	related, err := ie.queryByColumn(ctx, rel.RelatedMeta, rel.RelatedMeta.PKColumn, fkValues, inc)
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

const includeBatchSize = 1000

// queryByColumn executes SELECT * FROM table WHERE column IN (values...)
// batching the IN list to stay within Postgres parameter limits.
// When inc.unscoped is false, meta.Filters are applied to the query.
// Any inc.wheres, inc.orderBy, and inc.limit are also applied.
func (ie *includeExecutor) queryByColumn(ctx context.Context, meta *ModelMetaBase, column string, values []any, inc IncludeSpec) ([]any, error) {
	if len(values) == 0 {
		return nil, nil
	}

	var allItems []any
	for i := 0; i < len(values); i += includeBatchSize {
		end := i + includeBatchSize
		if end > len(values) {
			end = len(values)
		}
		batch := values[i:end]

		inClause := ast.WhereClause{
			Comparison: &ast.ComparisonNode{
				Column: column,
				Op:     ast.OpIn,
				Values: batch,
			},
		}

		var where *ast.WhereClause
		hasFilters := !inc.unscoped && len(meta.Filters) > 0
		hasIncWheres := len(inc.wheres) > 0
		if hasFilters || hasIncWheres {
			capacity := 1 // inClause
			if hasFilters {
				capacity += len(meta.Filters)
			}
			capacity += len(inc.wheres)
			allWheres := make([]ast.WhereClause, 0, capacity)
			if hasFilters {
				for _, f := range meta.Filters {
					allWheres = append(allWheres, f.Clause)
				}
			}
			allWheres = append(allWheres, inClause)
			allWheres = append(allWheres, inc.wheres...)
			combined := ast.WhereClause{
				LogicalOp: ast.LogicalAnd,
				Children:  allWheres,
			}
			where = &combined
		} else {
			where = &inClause
		}

		node := ast.SelectNode{
			Table:   meta.Table,
			Columns: meta.Columns,
			Where:   where,
			OrderBy: inc.orderBy,
			Limit:   inc.limit,
			Type:    ast.QuerySelect,
		}

		result := ie.engine.dialect().BuildSelect(node)
		rows, err := ie.engine.queryInternal(ctx, result.SQL, result.Args...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			item, err := meta.ScanRow(rows)
			if err != nil {
				rows.Close()
				return nil, err
			}
			allItems = append(allItems, item)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return allItems, nil
}

// loadManyToMany loads related entities through a pivot table in 3 steps:
// 1. SELECT source_fk, target_fk FROM pivot WHERE source_fk IN (...)
// 2. SELECT * FROM target WHERE pk IN (collected target PKs)
// 3. Build source→[]target mapping and assign via FieldSetter
func (ie *includeExecutor) loadManyToMany(ctx context.Context, parents []any, inc IncludeSpec) error {
	rel := inc.rel
	pkValues := make([]any, len(parents))
	for i, p := range parents {
		pkValues[i] = ie.parentMeta.PKValue(p)
	}

	type pivotRow struct {
		sourceFK, targetFK any
	}
	var pivotRows []pivotRow

	for i := 0; i < len(pkValues); i += includeBatchSize {
		end := i + includeBatchSize
		if end > len(pkValues) {
			end = len(pkValues)
		}
		batch := pkValues[i:end]

		node := ast.SelectNode{
			Table:   rel.JoinTable,
			Columns: []string{rel.FKColumn, rel.RefColumn},
			Where: &ast.WhereClause{
				Comparison: &ast.ComparisonNode{
					Column: rel.FKColumn,
					Op:     ast.OpIn,
					Values: batch,
				},
			},
			Type: ast.QuerySelect,
		}

		result := ie.engine.dialect().BuildSelect(node)
		rows, err := ie.engine.queryInternal(ctx, result.SQL, result.Args...)
		if err != nil {
			return err
		}

		for rows.Next() {
			var src, tgt any
			if err := rows.Scan(&src, &tgt); err != nil {
				rows.Close()
				return err
			}
			// Normalize int64 → int so map lookups match PKValue which returns Go int.
			pivotRows = append(pivotRows, pivotRow{sourceFK: normalizeInt(src), targetFK: normalizeInt(tgt)})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}

	if len(pivotRows) == 0 {
		for _, p := range parents {
			rel.FieldSetter(p, []any{})
		}
		return nil
	}

	seen := make(map[any]bool)
	var targetPKs []any
	for _, pr := range pivotRows {
		if !seen[pr.targetFK] {
			seen[pr.targetFK] = true
			targetPKs = append(targetPKs, pr.targetFK)
		}
	}

	targets, err := ie.queryByColumn(ctx, rel.RelatedMeta, rel.RelatedMeta.PKColumn, targetPKs, inc)
	if err != nil {
		return err
	}

	targetByPK := make(map[any]any)
	for _, t := range targets {
		pk := rel.RelatedMeta.PKValue(t)
		targetByPK[pk] = t
	}

	grouped := make(map[any][]any)
	for _, pr := range pivotRows {
		if t, ok := targetByPK[pr.targetFK]; ok {
			grouped[pr.sourceFK] = append(grouped[pr.sourceFK], t)
		}
	}

	for _, p := range parents {
		pk := ie.parentMeta.PKValue(p)
		items := grouped[pk]
		if items == nil {
			items = []any{}
		}
		rel.FieldSetter(p, items)
	}

	return nil
}

// normalizeInt converts int64/int32 to int so that map lookups match
// PKValue functions that return Go int. pgx scans INTEGER columns as int64
// when the target is any, causing type mismatches in map keys.
func normalizeInt(v any) any {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int32:
		return int(n)
	default:
		return v
	}
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
