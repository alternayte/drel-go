package drel

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alternayte/drel/internal/dberr"
	"github.com/alternayte/drel/internal/driver"
)

type flakyCommitTx struct{ remaining *int }

func (t *flakyCommitTx) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	return nil
}
func (t *flakyCommitTx) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	return nil, nil
}
func (t *flakyCommitTx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (t *flakyCommitTx) Commit(ctx context.Context) error {
	if *t.remaining > 0 {
		*t.remaining--
		return errors.New("database is locked") // classifies to ErrSerializationFailure
	}
	return nil
}
func (t *flakyCommitTx) Rollback(ctx context.Context) error { return nil }

type flakyCommitDriver struct {
	remaining *int
	begins    int
}

func (d *flakyCommitDriver) QueryRow(ctx context.Context, sql string, args ...any) driver.Row {
	return nil
}
func (d *flakyCommitDriver) Query(ctx context.Context, sql string, args ...any) (driver.Rows, error) {
	return nil, nil
}
func (d *flakyCommitDriver) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	return 0, nil
}
func (d *flakyCommitDriver) Begin(ctx context.Context) (driver.Tx, error) {
	d.begins++
	return &flakyCommitTx{remaining: d.remaining}, nil
}
func (d *flakyCommitDriver) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	d.begins++
	return &flakyCommitTx{remaining: d.remaining}, nil
}
func (d *flakyCommitDriver) Ping(ctx context.Context) error { return nil }
func (d *flakyCommitDriver) Stat() driver.PoolStat          { return driver.PoolStat{} }
func (d *flakyCommitDriver) Close()                         {}

func TestTransactionWithRetry_RetriesUntilSuccess(t *testing.T) {
	rem := 2 // fail twice, then succeed on attempt 3
	drv := &flakyCommitDriver{remaining: &rem}
	e := &Engine{drv: drv}
	calls := 0
	err := e.TransactionWithRetry(context.Background(),
		func(tx *Tx) error { calls++; return nil },
		WithRetry(RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 5 * time.Millisecond}),
	)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if drv.begins != 3 {
		t.Fatalf("expected 3 transaction attempts, got %d", drv.begins)
	}
	if calls != 3 {
		t.Fatalf("fn should run once per attempt (3), got %d", calls)
	}
}

func TestTransactionWithRetry_ExhaustsAndReturnsSerializationError(t *testing.T) {
	rem := 99 // always fail
	drv := &flakyCommitDriver{remaining: &rem}
	e := &Engine{drv: drv}
	err := e.TransactionWithRetry(context.Background(),
		func(tx *Tx) error { return nil },
		WithRetry(RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}),
	)
	if !errors.Is(err, dberr.ErrSerializationFailure) {
		t.Fatalf("exhausted retries should return the last ErrSerializationFailure, got %v", err)
	}
	if drv.begins != 3 {
		t.Fatalf("expected exactly MaxAttempts=3 attempts, got %d", drv.begins)
	}
}

func TestTransactionWithRetry_NonSerializationReturnsImmediately(t *testing.T) {
	rem := 0
	drv := &flakyCommitDriver{remaining: &rem}
	e := &Engine{drv: drv}
	sentinel := errors.New("boom")
	err := e.TransactionWithRetry(context.Background(),
		func(tx *Tx) error { return sentinel },
		WithRetry(RetryConfig{MaxAttempts: 3, BaseDelay: time.Millisecond}),
	)
	if !errors.Is(err, sentinel) {
		t.Fatalf("non-serialization error must return immediately, got %v", err)
	}
	if drv.begins != 1 {
		t.Fatalf("must not retry a non-serialization error, attempts=%d", drv.begins)
	}
}

func TestTransactionWithRetry_HonoursCtxCancellation(t *testing.T) {
	rem := 99
	drv := &flakyCommitDriver{remaining: &rem}
	e := &Engine{drv: drv}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the first backoff
	err := e.TransactionWithRetry(ctx,
		func(tx *Tx) error { return nil },
		WithRetry(RetryConfig{MaxAttempts: 5, BaseDelay: 50 * time.Millisecond}),
	)
	// First attempt runs and fails serialization; the backoff wait observes the
	// cancelled ctx and returns context.Canceled.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ctx during backoff should return context.Canceled, got %v", err)
	}
	if drv.begins > 1 {
		t.Fatalf("must not start a new attempt after ctx cancellation, attempts=%d", drv.begins)
	}
}

func TestTransactionWithRetry_DefaultsWhenNoConfig(t *testing.T) {
	rem := 1 // one failure, then succeed — within the default MaxAttempts (3)
	drv := &flakyCommitDriver{remaining: &rem}
	e := &Engine{drv: drv}
	if err := e.TransactionWithRetry(context.Background(), func(tx *Tx) error { return nil }); err != nil {
		t.Fatalf("default retry config should recover from one failure, got %v", err)
	}
	if drv.begins != 2 {
		t.Fatalf("expected 2 attempts under defaults, got %d", drv.begins)
	}
}
