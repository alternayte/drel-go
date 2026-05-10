package drel

import (
	"context"
	"fmt"
)

// SetClause pairs a column name with a value for use in bulk updates and upserts.
type SetClause struct {
	Column string
	Value  any
}

// Set creates a SetClause from a typed column and value.
func Set[C interface{ Name() string }, V any](col C, value V) SetClause {
	return SetClause{Column: col.Name(), Value: value}
}

// UpsertOption configures BulkUpsert behavior.
type UpsertOption func(*upsertConfig)

type upsertConfig struct {
	conflictCols []string
	updateCols   []string
}

// ConflictColumns specifies which columns form the conflict target for an upsert.
func ConflictColumns[C interface{ Name() string }](cols ...C) UpsertOption {
	return func(cfg *upsertConfig) {
		for _, c := range cols {
			cfg.conflictCols = append(cfg.conflictCols, c.Name())
		}
	}
}

// UpdateOnConflict specifies which columns to update when a conflict is detected.
func UpdateOnConflict[C interface{ Name() string }](cols ...C) UpsertOption {
	return func(cfg *upsertConfig) {
		for _, c := range cols {
			cfg.updateCols = append(cfg.updateCols, c.Name())
		}
	}
}

const bulkBatchSize = 1000

// BulkInsert inserts multiple entities in batches, bypassing change tracking.
// Returns the total number of rows affected.
func (r *Repository[T]) BulkInsert(ctx context.Context, entities []*T) (int, error) {
	if len(entities) == 0 {
		return 0, nil
	}

	drv := r.engine.Driver()
	d := r.engine.Dialect()

	tx, err := drv.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk insert begin: %w", err)
	}

	total := 0
	for i := 0; i < len(entities); i += bulkBatchSize {
		end := i + bulkBatchSize
		if end > len(entities) {
			end = len(entities)
		}
		batch := entities[i:end]

		var columns []string
		var rows [][]any
		for j, entity := range batch {
			cols, vals := r.meta.InsertColumns(entity)
			if j == 0 {
				columns = cols
			}
			rows = append(rows, vals)
		}

		result := d.BuildBulkInsert(r.meta.Table, columns, rows)
		affected, execErr := tx.Exec(ctx, result.SQL, result.Args...)
		if execErr != nil {
			_ = tx.Rollback(ctx)
			return total, fmt.Errorf("drel: bulk insert %s: %w", r.meta.Table, execErr)
		}
		total += int(affected)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("drel: bulk insert commit: %w", err)
	}

	return total, nil
}

// BulkUpsert inserts or updates multiple entities based on conflict resolution.
// It bypasses change tracking and executes directly against the database.
// Returns the total number of rows affected.
func (r *Repository[T]) BulkUpsert(ctx context.Context, entities []*T, opts ...UpsertOption) (int, error) {
	if len(entities) == 0 {
		return 0, nil
	}

	cfg := &upsertConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	d := r.engine.Dialect()

	var columns []string
	var rows [][]any
	for i, entity := range entities {
		cols, vals := r.meta.InsertColumns(entity)
		if i == 0 {
			columns = cols
		}
		rows = append(rows, vals)
	}

	result := d.BuildBulkUpsert(r.meta.Table, columns, rows, cfg.conflictCols, cfg.updateCols)
	affected, err := r.engine.Driver().Exec(ctx, result.SQL, result.Args...)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk upsert %s: %w", r.meta.Table, err)
	}

	return int(affected), nil
}
