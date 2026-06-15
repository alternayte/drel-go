package drel

import (
	"context"
	"fmt"
	"time"

	"github.com/alternayte/drel/internal/dberr"
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

// maxBulkParams is a conservative cap on bound parameters per statement, safe
// across dialects: SQLite's default SQLITE_MAX_VARIABLE_NUMBER is 32766 and
// Postgres allows 65535. Staying under the smaller keeps wide-table bulk
// operations from overflowing the limit.
const maxBulkParams = 32766

// safeBatchSize returns the largest row count whose parameter total stays within
// maxBulkParams, capped at bulkBatchSize. numCols is the bound parameters per row.
func safeBatchSize(numCols int) int {
	if numCols <= 0 {
		return bulkBatchSize
	}
	n := maxBulkParams / numCols
	if n < 1 {
		n = 1
	}
	if n > bulkBatchSize {
		n = bulkBatchSize
	}
	return n
}

// appendUniformRow validates that an entity's insert columns match the batch's
// established column list (same names, same order, same length) and appends its
// values. The first entity establishes `columns`. A mismatch is a loud error
// rather than a silent positional bind against the wrong columns.
func appendUniformRow(table string, columns *[]string, rows *[][]any, cols []string, vals []any) error {
	if len(cols) != len(vals) {
		return fmt.Errorf("drel: bulk %s: InsertColumns returned %d columns but %d values", table, len(cols), len(vals))
	}
	if *columns == nil {
		*columns = cols
	} else {
		if len(cols) != len(*columns) {
			return fmt.Errorf("drel: bulk %s: non-uniform column shape (%d vs %d columns); InsertColumns must return a fixed column set", table, len(cols), len(*columns))
		}
		for i := range cols {
			if cols[i] != (*columns)[i] {
				return fmt.Errorf("drel: bulk %s: non-uniform column shape (column %d %q vs %q); InsertColumns must return a fixed column set", table, i, cols[i], (*columns)[i])
			}
		}
	}
	*rows = append(*rows, vals)
	return nil
}

// BulkInsert inserts multiple entities in batches, bypassing change tracking.
// Returns the total number of rows affected.
func (r *Repository[T]) BulkInsert(ctx context.Context, entities []*T) (int, error) {
	if len(entities) == 0 {
		return 0, nil
	}

	drv := r.engine.driver()
	d := r.engine.dialect()

	tx, err := drv.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk insert begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	firstCols, _ := r.meta.InsertColumns(entities[0])
	batchSize := safeBatchSize(len(firstCols))

	total := 0
	for i := 0; i < len(entities); i += batchSize {
		end := i + batchSize
		if end > len(entities) {
			end = len(entities)
		}
		batch := entities[i:end]

		var columns []string
		var rows [][]any
		for _, entity := range batch {
			cols, vals := r.meta.InsertColumns(entity)
			if err := appendUniformRow(r.meta.Table, &columns, &rows, cols, vals); err != nil {
				return 0, err
			}
		}

		result := d.BuildBulkInsert(r.meta.Table, columns, rows)
		start := time.Now()
		affected, execErr := tx.Exec(ctx, result.SQL, result.Args...)
		r.engine.notifyQueryHooks(ctx, result.SQL, result.Args, time.Since(start), execErr)
		if execErr != nil {
			return total, fmt.Errorf("drel: bulk insert %s: %w", r.meta.Table, dberr.Classify(execErr))
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
// ErrBulkDeleteRequiresFilter is returned when BulkDelete is called without any WHERE predicates or filters.
var ErrBulkDeleteRequiresFilter = fmt.Errorf("drel: BulkDelete requires at least one Where predicate to prevent accidental full-table deletes")

func (r *Repository[T]) BulkUpsert(ctx context.Context, entities []*T, opts ...UpsertOption) (int, error) {
	if len(entities) == 0 {
		return 0, nil
	}

	cfg := &upsertConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	if len(cfg.conflictCols) == 0 {
		return 0, fmt.Errorf("drel: bulk upsert %s: ConflictColumns is required", r.meta.Table)
	}
	if len(cfg.updateCols) == 0 {
		return 0, fmt.Errorf("drel: bulk upsert %s: UpdateOnConflict is required", r.meta.Table)
	}

	drv := r.engine.driver()
	d := r.engine.dialect()

	tx, err := drv.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("drel: bulk upsert begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	firstCols, _ := r.meta.InsertColumns(entities[0])
	batchSize := safeBatchSize(len(firstCols))

	total := 0
	for i := 0; i < len(entities); i += batchSize {
		end := i + batchSize
		if end > len(entities) {
			end = len(entities)
		}
		batch := entities[i:end]

		var columns []string
		var rows [][]any
		for _, entity := range batch {
			cols, vals := r.meta.InsertColumns(entity)
			if err := appendUniformRow(r.meta.Table, &columns, &rows, cols, vals); err != nil {
				return 0, err
			}
		}

		result := d.BuildBulkUpsert(r.meta.Table, columns, rows, cfg.conflictCols, cfg.updateCols)
		start := time.Now()
		affected, execErr := tx.Exec(ctx, result.SQL, result.Args...)
		r.engine.notifyQueryHooks(ctx, result.SQL, result.Args, time.Since(start), execErr)
		if execErr != nil {
			return total, fmt.Errorf("drel: bulk upsert %s: %w", r.meta.Table, dberr.Classify(execErr))
		}
		total += int(affected)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("drel: bulk upsert commit: %w", err)
	}

	return total, nil
}
