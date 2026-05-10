package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/dialect"
	"github.com/alternayte/drel/internal/driver"
)

func flushChanges(ctx context.Context, dbTx driver.Tx, d dialect.Dialect, tracker *changeTracker) ([]any, error) {
	tracker.DetectChanges()
	pc := tracker.GetPendingChanges()

	for _, te := range pc.Added {
		if te.meta.HasAudit && te.meta.AuditSetCreate != nil {
			actor := ActorFromContext(ctx)
			te.meta.AuditSetCreate(te.entity, actor)
		}
		cols, vals := te.meta.InsertColumns(te.entity)
		result := d.BuildInsert(te.meta.Table, cols, vals, []string{"id", "created_at", "updated_at"})
		row := dbTx.QueryRow(ctx, result.SQL, result.Args...)
		if te.meta.ScanReturning != nil {
			if err := te.meta.ScanReturning(te.entity, row); err != nil {
				return nil, fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
			}
		} else {
			var discard any
			if err := row.Scan(&discard, &discard, &discard); err != nil {
				return nil, fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
			}
		}
		if te.meta.HasVersioned && te.meta.SetVersion != nil {
			te.meta.SetVersion(te.entity, 1)
		}
	}

	for _, te := range pc.Modified {
		if te.meta.HasAudit && te.meta.AuditSetUpdate != nil {
			actor := ActorFromContext(ctx)
			te.meta.AuditSetUpdate(te.entity, actor)
		}
		changes := te.meta.Diff(te.entity, te.snapshot)
		if len(changes) == 0 {
			continue
		}
		if te.meta.HasAudit {
			actor := ActorFromContext(ctx)
			changes = append(changes, FieldChange{Column: "updated_by", Value: actor})
		}
		cvs := make([]dialect.ColumnValue, len(changes))
		for i, c := range changes {
			cvs[i] = dialect.ColumnValue{Column: c.Column, Value: c.Value}
		}
		pkVal := te.meta.PKValue(te.entity)

		if te.meta.HasVersioned && te.meta.VersionValue != nil {
			currentVersion := te.meta.VersionValue(te.entity)
			result := d.BuildUpdateVersioned(te.meta.Table, cvs, te.meta.PKColumn, pkVal, "version", currentVersion)
			row := dbTx.QueryRow(ctx, result.SQL, result.Args...)
			var newVersion int
			if err := row.Scan(&newVersion); err != nil {
				return nil, ErrConcurrencyConflict
			}
			te.meta.SetVersion(te.entity, newVersion)
		} else {
			result := d.BuildUpdate(te.meta.Table, cvs, te.meta.PKColumn, pkVal)
			affected, err := dbTx.Exec(ctx, result.SQL, result.Args...)
			if err != nil {
				return nil, fmt.Errorf("drel: update %s: %w", te.meta.Table, err)
			}
			if affected == 0 {
				return nil, fmt.Errorf("drel: update %s: no rows affected (pk=%v)", te.meta.Table, pkVal)
			}
		}
	}

	for _, te := range pc.Deleted {
		pkVal := te.meta.PKValue(te.entity)
		if te.meta.HasSoftDelete && !te.hardDelete {
			result := d.BuildSoftDelete(te.meta.Table, te.meta.PKColumn, pkVal)
			_, err := dbTx.Exec(ctx, result.SQL, result.Args...)
			if err != nil {
				return nil, fmt.Errorf("drel: soft delete %s: %w", te.meta.Table, err)
			}
		} else {
			result := d.BuildDelete(te.meta.Table, te.meta.PKColumn, pkVal)
			_, err := dbTx.Exec(ctx, result.SQL, result.Args...)
			if err != nil {
				return nil, fmt.Errorf("drel: delete %s: %w", te.meta.Table, err)
			}
		}
	}

	events := collectPendingEvents(tracker)
	tracker.PostFlush()
	return events, nil
}

func flushHookChanges(ctx context.Context, dbTx driver.Tx, d dialect.Dialect, tracker *changeTracker) error {
	tracker.DetectChanges()
	pc := tracker.GetPendingChanges()

	for _, te := range pc.Added {
		cols, vals := te.meta.InsertColumns(te.entity)
		result := d.BuildInsert(te.meta.Table, cols, vals, []string{"id", "created_at", "updated_at"})
		row := dbTx.QueryRow(ctx, result.SQL, result.Args...)
		if te.meta.ScanReturning != nil {
			if err := te.meta.ScanReturning(te.entity, row); err != nil {
				return fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
			}
		} else {
			var discard any
			if err := row.Scan(&discard, &discard, &discard); err != nil {
				return fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
			}
		}
	}

	tracker.PostFlush()
	return nil
}

func collectPendingEvents(tracker *changeTracker) []any {
	var events []any
	for _, te := range tracker.entities {
		if er, ok := te.entity.(EventRecorder); ok {
			events = append(events, er.PendingEvents()...)
			er.ClearEvents()
		}
	}
	return events
}
