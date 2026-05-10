package drel

import (
	"context"
	"fmt"
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
}

// WithIsolation sets the isolation level for the transaction.
func WithIsolation(level IsolationLevel) TxOption {
	return func(cfg *txConfig) {
		cfg.isolation = &level
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

	dbTx, err := e.drv.Begin(ctx)
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
			_ = dbTx.Rollback(ctx)
			panic(p)
		}
		if retErr != nil {
			_ = dbTx.Rollback(ctx)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}

	events, err := flushChanges(ctx, dbTx, e.Dialect(), tx.tracker)
	if err != nil {
		return err
	}
	allEvents := append(tx.heldEvents, events...)

	for _, hook := range e.beforeCommitHooks {
		if err := hook(ctx, tx, allEvents); err != nil {
			return err
		}
	}

	if err := flushHookChanges(ctx, dbTx, e.Dialect(), tx.tracker); err != nil {
		return err
	}

	if err := dbTx.Commit(ctx); err != nil {
		return fmt.Errorf("drel: commit: %w", err)
	}

	for _, hook := range e.afterCommitHooks {
		hook(ctx, allEvents)
	}

	return nil
}
