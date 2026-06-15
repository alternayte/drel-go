package drel

import (
	"context"
	"fmt"

	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/driver"
)

// IsolationLevel represents the transaction isolation level.
type IsolationLevel int

const (
	// ReadCommitted is the default isolation level.
	ReadCommitted IsolationLevel = iota
	// RepeatableRead prevents non-repeatable reads.
	RepeatableRead
	// Serializable provides full serializability.
	Serializable
)

// TxOption configures transaction behaviour.
type TxOption func(*txConfig)

type txConfig struct {
	isolation *IsolationLevel
	readOnly  bool
	retry     *RetryConfig
}

// WithIsolation sets the isolation level for the transaction.
func WithIsolation(level IsolationLevel) TxOption {
	return func(cfg *txConfig) {
		cfg.isolation = &level
	}
}

// WithReadOnly begins the transaction in read-only mode. On Postgres this lets
// the server skip snapshot bookkeeping and rejects any write inside the tx; on
// SQLite/libSQL the flag is forwarded to the driver. Compose it with
// WithIsolation; both reach the driver's BeginTx.
func WithReadOnly() TxOption {
	return func(cfg *txConfig) {
		cfg.readOnly = true
	}
}

// Transaction runs fn inside a database transaction. If fn returns nil the
// transaction is committed; otherwise it is rolled back. Pending changes in
// the Tx change tracker are flushed automatically before commit.
func (e *Engine) Transaction(ctx context.Context, fn func(tx *Tx) error, opts ...TxOption) (retErr error) {
	cfg := &txConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("drel: transaction: %w", err)
	}

	var dbTx driver.Tx
	var err error
	if cfg.isolation != nil || cfg.readOnly {
		drvIso := driver.IsoDefault
		if cfg.isolation != nil {
			switch *cfg.isolation {
			case ReadCommitted:
				drvIso = driver.IsoReadCommitted
			case RepeatableRead:
				drvIso = driver.IsoRepeatableRead
			case Serializable:
				drvIso = driver.IsoSerializable
			}
		}
		dbTx, err = e.drv.BeginTx(ctx, driver.TxOptions{Isolation: drvIso, ReadOnly: cfg.readOnly})
	} else {
		dbTx, err = e.drv.Begin(ctx)
	}
	if err != nil {
		return fmt.Errorf("drel: begin transaction: %w", err)
	}

	tx := &Tx{
		engine:  e,
		dbTx:    dbTx,
		tracker: newChangeTracker(),
	}

	defer func() {
		if p := recover(); p != nil {
			_ = dbTx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if retErr != nil {
			_ = dbTx.Rollback(context.WithoutCancel(ctx))
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	events, err := flushChanges(ctx, tx, e.dialect(), tx.tracker)
	if err != nil {
		tx.tracker.resetFlushed()
		return err
	}
	allEvents := append(tx.heldEvents, events...)

	for _, hook := range e.snapshotBeforeCommitHooks() {
		if err := hook(ctx, tx, allEvents); err != nil {
			tx.tracker.resetFlushed()
			return err
		}
	}

	hookEvents, err := flushHookChanges(ctx, tx, e.dialect(), tx.tracker)
	if err != nil {
		tx.tracker.resetFlushed()
		return err
	}
	allEvents = append(allEvents, hookEvents...)

	for _, sink := range e.eventSinks {
		if err := sink(ctx, tx, allEvents); err != nil {
			tx.tracker.resetFlushed()
			return err
		}
	}

	if e.devMode {
		if n := tx.tracker.countUnusedTracked(); n > 0 {
			e.devWarn(ctx, "drel dev: tracked entities were loaded but never modified; consider AsNoTracking for read-only queries",
				"count", n)
		}
	}

	if err := dbTx.Commit(ctx); err != nil {
		tx.tracker.resetFlushed()
		return fmt.Errorf("drel: commit: %w", dberr.Classify(err))
	}
	tx.tracker.PostCommit()
	clearPendingEvents(tx.tracker)

	e.dispatchAfterCommit(ctx, allEvents)

	return nil
}
