package drel

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/alternayte/drel/internal/dberr"
)

// Default retry bounds used when a field of RetryConfig is left zero.
const (
	defaultRetryAttempts  = 3
	defaultRetryBaseDelay = 50 * time.Millisecond
	defaultRetryMaxDelay  = time.Second
)

// RetryConfig bounds serialization-failure retries for TransactionWithRetry.
// Zero fields fall back to sensible defaults (3 attempts, 50ms base delay
// growing exponentially with jitter, capped at 1s).
type RetryConfig struct {
	// MaxAttempts is the total number of tries, including the first. <=0 means 3.
	MaxAttempts int
	// BaseDelay is the first backoff delay; subsequent delays grow exponentially.
	// <=0 means 50ms.
	BaseDelay time.Duration
	// MaxDelay caps the backoff delay. <=0 means 1s.
	MaxDelay time.Duration
}

func (c RetryConfig) normalized() RetryConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultRetryAttempts
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = defaultRetryBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = defaultRetryMaxDelay
	}
	return c
}

// WithRetry configures TransactionWithRetry to re-run the transaction on
// ErrSerializationFailure using the given bounds. It is read only by
// TransactionWithRetry; plain Transaction ignores it.
func WithRetry(cfg RetryConfig) TxOption {
	return func(tc *txConfig) { tc.retry = &cfg }
}

// TransactionWithRetry runs fn in a transaction, re-running it on
// ErrSerializationFailure (deadlock / SQLSTATE 40001 / SQLITE_BUSY) up to
// MaxAttempts, with exponential backoff + jitter, honouring ctx cancellation
// between attempts. Configure the bounds with WithRetry; without it, defaults
// apply (3 attempts). Each attempt gets a fresh Tx and change tracker, so fn
// MUST rebuild any entities it adds/edits on every call — do not reuse entity
// pointers across attempts (see the savepoint in-memory caveat on Tx.Savepoint).
//
// Any non-serialization error (or success) returns immediately.
func (e *Engine) TransactionWithRetry(ctx context.Context, fn func(tx *Tx) error, opts ...TxOption) error {
	// Read the retry config from opts without disturbing the TxOptions passed
	// through to Transaction (WithRetry is a no-op inside Transaction itself).
	cfg := &txConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	rc := RetryConfig{}
	if cfg.retry != nil {
		rc = *cfg.retry
	}
	rc = rc.normalized()

	var err error
	for attempt := 1; ; attempt++ {
		err = e.Transaction(ctx, fn, opts...)
		if err == nil || !errors.Is(err, dberr.ErrSerializationFailure) {
			return err
		}
		if attempt >= rc.MaxAttempts {
			return err
		}
		if waitErr := sleepBackoff(ctx, rc, attempt); waitErr != nil {
			return waitErr
		}
	}
}

// sleepBackoff waits for an exponentially growing, jittered delay or returns the
// ctx error if it is cancelled first.
func sleepBackoff(ctx context.Context, rc RetryConfig, attempt int) error {
	delay := rc.BaseDelay << (attempt - 1) // 2^(attempt-1) * base
	if delay <= 0 || delay > rc.MaxDelay { // overflow or over cap
		delay = rc.MaxDelay
	}
	// Full jitter in [0, delay].
	if delay > 0 {
		delay = time.Duration(rand.Int63n(int64(delay) + 1))
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
