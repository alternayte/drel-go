//go:build integration

package drel_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/require"
)

// TestIntegration_CommitTimeSerializationClassified provokes a real SERIALIZABLE
// 40001 that fires at COMMIT (the case plain exec-classification misses) and
// asserts it surfaces as ErrSerializationFailure.
func TestIntegration_CommitTimeSerializationClassified(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `CREATE TABLE acct (id INT PRIMARY KEY, bal INT NOT NULL)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO acct (id, bal) VALUES (1, 100), (2, 100)`)
	require.NoError(t, err)

	// Two SERIALIZABLE transactions that read both rows then update opposite rows
	// — a classic write-skew that Postgres aborts at COMMIT with 40001.
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	run := func(idx, updateID int) {
		defer wg.Done()
		errs[idx] = engine.Transaction(ctx, func(tx *drel.Tx) error {
			if _, e := tx.Exec(ctx, `SELECT bal FROM acct WHERE id = 1`); e != nil {
				return e
			}
			if _, e := tx.Exec(ctx, `SELECT bal FROM acct WHERE id = 2`); e != nil {
				return e
			}
			<-start // both read before either writes
			_, e := tx.Exec(ctx, `UPDATE acct SET bal = bal - 10 WHERE id = $1`, updateID)
			return e
		}, drel.WithIsolation(drel.Serializable))
	}
	wg.Add(2)
	go run(0, 1)
	go run(1, 2)
	time.Sleep(100 * time.Millisecond)
	close(start)
	wg.Wait()

	// At least one transaction must fail, and that failure must classify.
	got := errs[0]
	if got == nil {
		got = errs[1]
	}
	require.Error(t, got, "one SERIALIZABLE txn should have failed with 40001")
	require.True(t, errors.Is(got, drel.ErrSerializationFailure),
		"serialization failure (incl. commit-time) must classify, got %v", got)
}

// TestIntegration_TransactionWithRetry_ResolvesContention asserts the retry
// helper drives the same write-skew to eventual success.
func TestIntegration_TransactionWithRetry_ResolvesContention(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `CREATE TABLE ctr (id INT PRIMARY KEY, n INT NOT NULL)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO ctr (id, n) VALUES (1, 0)`)
	require.NoError(t, err)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			e := engine.TransactionWithRetry(ctx, func(tx *drel.Tx) error {
				row := tx.QueryRow(ctx, `SELECT n FROM ctr WHERE id = 1`)
				var n int
				if err := row.Scan(&n); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `UPDATE ctr SET n = $1 WHERE id = 1`, n+1)
				return err
			}, drel.WithIsolation(drel.Serializable),
				drel.WithRetry(drel.RetryConfig{MaxAttempts: 20, BaseDelay: 2 * time.Millisecond, MaxDelay: 50 * time.Millisecond}))
			require.NoError(t, e)
		}()
	}
	wg.Wait()

	row := engine.QueryRow(ctx, `SELECT n FROM ctr WHERE id = 1`)
	var n int
	require.NoError(t, row.Scan(&n))
	require.Equal(t, workers, n, "every increment must commit exactly once under retry")
}
