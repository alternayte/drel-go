package drel

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/alternayte/drel/internal/dialect"
)

type txExec interface {
	execInternal(ctx context.Context, sql string, args ...any) (int64, error)
	queryRowInternal(ctx context.Context, sql string, args ...any) Row
}

// isNoRows returns true if err represents a "no rows in result set" error.
// This handles both database/sql's sql.ErrNoRows and pgx's own ErrNoRows
// (whose message is "no rows in result set") without importing pgx.
func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	return err.Error() == "no rows in result set"
}

// quoteIdentMutation quotes a SQL identifier for use in mutation readback queries.
func quoteIdentMutation(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// applyPendingChanges performs DetectChanges + GetPendingChanges and executes all
// the necessary INSERTs, UPDATEs, and DELETEs against the database. It marks
// each flushed entity with flushed=true and returns the slice of entities that
// were actually flushed so callers can decide how to collect events from them.
func applyPendingChanges(ctx context.Context, exec txExec, d dialect.Dialect, tracker *changeTracker) ([]*trackedEntity, error) {
	tracker.DetectChanges()
	pc := tracker.GetPendingChanges()

	for _, te := range pc.Added {
		if te.meta.HasAudit && te.meta.AuditSetCreate != nil {
			actor := ActorFromContext(ctx)
			te.meta.AuditSetCreate(te.entity, actor)
		}
		cols, vals := te.meta.InsertColumns(te.entity)

		appAssigned := te.meta.KeyStrategy == KeyAppAssigned
		if appAssigned {
			// An app-assigned key must be set by now (by a generator at Add time or
			// by the application). A zero key means it was forgotten — fail loudly
			// rather than persisting a zero/empty primary key.
			if te.meta.KeyIsZero != nil && te.meta.KeyIsZero(te.entity) {
				return nil, fmt.Errorf("drel: insert %s: app-assigned primary key is zero (no key generator registered and no key was set)", te.meta.Table)
			}
			// Include the (already-stamped) PK in the INSERT and read back only
			// the DB-generated timestamps — never the id.
			cols = append([]string{te.meta.PKColumn}, cols...)
			vals = append([]any{te.meta.PKValue(te.entity)}, vals...)
		}

		if d.SupportsReturning() {
			returning := []string{"id", "created_at", "updated_at"}
			scan := te.meta.ScanReturning
			if appAssigned {
				returning = []string{"created_at", "updated_at"}
				scan = te.meta.ScanGenerated
			}
			result := d.BuildInsert(te.meta.Table, cols, vals, returning)
			row := exec.queryRowInternal(ctx, result.SQL, result.Args...)
			if scan != nil {
				if err := scan(te.entity, row); err != nil {
					return nil, fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
				}
			} else {
				discards := make([]any, len(returning))
				for i := range discards {
					discards[i] = new(any)
				}
				if err := row.Scan(discards...); err != nil {
					return nil, fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
				}
			}
		} else {
			result := d.BuildInsert(te.meta.Table, cols, vals, nil)
			if _, err := exec.execInternal(ctx, result.SQL, result.Args...); err != nil {
				return nil, fmt.Errorf("drel: insert %s: %w", te.meta.Table, err)
			}
			if appAssigned {
				// Read back timestamps by the known id (correct for non-integer
				// keys). Skip entirely if the model has no timestamp scanner.
				if te.meta.ScanGenerated != nil {
					readbackSQL := fmt.Sprintf(
						`SELECT %s, %s FROM %s WHERE %s = ?`,
						quoteIdentMutation("created_at"),
						quoteIdentMutation("updated_at"),
						quoteIdentMutation(te.meta.Table),
						quoteIdentMutation(te.meta.PKColumn),
					)
					row := exec.queryRowInternal(ctx, readbackSQL, te.meta.PKValue(te.entity))
					if err := te.meta.ScanGenerated(te.entity, row); err != nil {
						return nil, fmt.Errorf("drel: insert readback %s: %w", te.meta.Table, err)
					}
				}
			} else {
				readbackSQL := fmt.Sprintf(
					`SELECT %s, %s, %s FROM %s WHERE rowid = last_insert_rowid()`,
					quoteIdentMutation("id"),
					quoteIdentMutation("created_at"),
					quoteIdentMutation("updated_at"),
					quoteIdentMutation(te.meta.Table),
				)
				row := exec.queryRowInternal(ctx, readbackSQL)
				if te.meta.ScanReturning != nil {
					if err := te.meta.ScanReturning(te.entity, row); err != nil {
						return nil, fmt.Errorf("drel: insert readback %s: %w", te.meta.Table, err)
					}
				} else {
					var discard any
					if err := row.Scan(&discard, &discard, &discard); err != nil {
						return nil, fmt.Errorf("drel: insert readback %s: %w", te.meta.Table, err)
					}
				}
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
		var changes []FieldChange
		if te.forceUpdate {
			// Entity was attached as Modified: no snapshot to diff against, so
			// UPDATE every user-settable column.
			cols, vals := te.meta.InsertColumns(te.entity)
			changes = make([]FieldChange, len(cols))
			for i, c := range cols {
				changes[i] = FieldChange{Column: c, Value: vals[i]}
			}
		} else {
			changes = te.meta.Diff(te.entity, te.snapshot)
		}
		if len(changes) == 0 {
			continue
		}
		changes = append(changes, FieldChange{Column: "updated_at", Value: RawExpr{SQL: d.Now()}})
		if te.meta.HasAudit {
			actor := ActorFromContext(ctx)
			changes = append(changes, FieldChange{Column: "updated_by", Value: actor})
		}
		cvs := make([]dialect.ColumnValue, len(changes))
		for i, c := range changes {
			val := c.Value
			if raw, ok := val.(RawExpr); ok {
				val = dialect.RawExpr{SQL: raw.SQL}
			}
			cvs[i] = dialect.ColumnValue{Column: c.Column, Value: val}
		}
		pkVal := te.meta.PKValue(te.entity)

		if te.meta.HasVersioned && te.meta.VersionValue != nil {
			currentVersion := te.meta.VersionValue(te.entity)
			result := d.BuildUpdateVersioned(te.meta.Table, cvs, te.meta.PKColumn, pkVal, "version", currentVersion)

			if d.SupportsReturning() {
				// Postgres path: UPDATE ... RETURNING version
				row := exec.queryRowInternal(ctx, result.SQL, result.Args...)
				var newVersion int
				if err := row.Scan(&newVersion); err != nil {
					if isNoRows(err) {
						return nil, ErrConcurrencyConflict
					}
					return nil, fmt.Errorf("drel: versioned update %s: %w", te.meta.Table, err)
				}
				te.meta.SetVersion(te.entity, newVersion)
			} else {
				// SQLite path: UPDATE without RETURNING, check affected rows.
				affected, err := exec.execInternal(ctx, result.SQL, result.Args...)
				if err != nil {
					return nil, fmt.Errorf("drel: versioned update %s: %w", te.meta.Table, err)
				}
				if affected == 0 {
					return nil, ErrConcurrencyConflict
				}
				// Version was incremented in the UPDATE SET clause (version = version + 1).
				te.meta.SetVersion(te.entity, currentVersion+1)
			}
		} else {
			result := d.BuildUpdate(te.meta.Table, cvs, te.meta.PKColumn, pkVal)
			affected, err := exec.execInternal(ctx, result.SQL, result.Args...)
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
			_, err := exec.execInternal(ctx, result.SQL, result.Args...)
			if err != nil {
				return nil, fmt.Errorf("drel: soft delete %s: %w", te.meta.Table, err)
			}
		} else {
			result := d.BuildDelete(te.meta.Table, te.meta.PKColumn, pkVal)
			_, err := exec.execInternal(ctx, result.SQL, result.Args...)
			if err != nil {
				return nil, fmt.Errorf("drel: delete %s: %w", te.meta.Table, err)
			}
		}
	}

	// Mark emitted entities flushed so a second flush in the same live
	// transaction does not re-emit. The tracker is finalized (PostCommit) by the
	// commit-owning wrapper only after a successful Commit.
	flushed := make([]*trackedEntity, 0, len(pc.Added)+len(pc.Modified)+len(pc.Deleted))
	for _, te := range pc.Added {
		te.flushed = true
		flushed = append(flushed, te)
	}
	for _, te := range pc.Modified {
		te.flushed = true
		flushed = append(flushed, te)
	}
	for _, te := range pc.Deleted {
		te.flushed = true
		flushed = append(flushed, te)
	}
	return flushed, nil
}

// flushChanges applies all pending changes to the database and returns the
// DELTA events — those belonging to the entities flushed in this call only.
// Callers accumulate delta events across multiple mid-transaction flushes via
// tx.heldEvents so no event is double-dispatched. Events are not cleared here;
// they are cleared post-commit by clearPendingEvents so a failed-then-retried
// SaveChanges can re-collect them.
func flushChanges(ctx context.Context, exec txExec, d dialect.Dialect, tracker *changeTracker) ([]any, error) {
	flushed, err := applyPendingChanges(ctx, exec, d, tracker)
	if err != nil {
		return nil, err
	}
	return eventsOf(flushed), nil
}

// flushHookChanges applies any changes staged by before-commit hooks (entities
// whose flushed flag is still false after the main flush) and returns ONLY the
// delta events — those belonging to the entities that were flushed in this call.
// Events are not cleared here; they are cleared post-commit by clearPendingEvents.
func flushHookChanges(ctx context.Context, exec txExec, d dialect.Dialect, tracker *changeTracker) ([]any, error) {
	flushed, err := applyPendingChanges(ctx, exec, d, tracker)
	if err != nil {
		return nil, err
	}
	return eventsOf(flushed), nil
}

// eventsOf collects the pending domain events from a specific set of tracked
// entities without clearing them. Used to build the delta event list after a
// hook flush so event-sinks (e.g. the outbox) receive the full combined set.
func eventsOf(entities []*trackedEntity) []any {
	var events []any
	for _, te := range entities {
		if er, ok := te.entity.(EventRecorder); ok {
			events = append(events, er.PendingEvents()...)
		}
	}
	return events
}

// collectPendingEvents gathers (without clearing) the pending domain events
// from every tracked entity. Events are cleared only after a successful commit
// (clearPendingEvents), so a failed-then-retried SaveChanges still finds them.
func collectPendingEvents(tracker *changeTracker) []any {
	var events []any
	for _, te := range tracker.entities {
		if er, ok := te.entity.(EventRecorder); ok {
			events = append(events, er.PendingEvents()...)
		}
	}
	return events
}

// clearPendingEvents clears recorded events from every tracked entity. Called
// on the post-commit path once events have been dispatched/persisted.
func clearPendingEvents(tracker *changeTracker) {
	for _, te := range tracker.entities {
		if er, ok := te.entity.(EventRecorder); ok {
			er.ClearEvents()
		}
	}
}
