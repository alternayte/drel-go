//go:build integration

package drel_test

import (
	"context"
	"sync"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// While transaction A holds advisory lock K, a concurrent transaction B's
// non-blocking TryAdvisoryLock(K) must report not-acquired; a different key
// must still succeed; and after A commits the lock is released.
func TestIntegration_AdvisoryLock_Contention(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	const key int64 = 999001

	heldStarted := make(chan struct{})
	releaseHold := make(chan struct{})
	var holderErr error
	var wg sync.WaitGroup
	wg.Add(1)

	// Transaction A: acquire the lock, signal, then wait before committing.
	go func() {
		defer wg.Done()
		holderErr = engine.Transaction(ctx, func(tx *drel.Tx) error {
			if err := tx.AdvisoryLock(ctx, key); err != nil {
				return err
			}
			close(heldStarted)
			<-releaseHold
			return nil
		})
	}()

	<-heldStarted

	// Transaction B: while A holds the lock, a try on the same key fails,
	// but a try on a different key succeeds.
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		acquired, tryErr := tx.TryAdvisoryLock(ctx, key)
		if tryErr != nil {
			return tryErr
		}
		assert.False(t, acquired, "expected contended TryAdvisoryLock to report not-acquired")

		other, otherErr := tx.TryAdvisoryLock(ctx, key+1)
		if otherErr != nil {
			return otherErr
		}
		assert.True(t, other, "expected uncontended TryAdvisoryLock to succeed")
		return nil
	})
	require.NoError(t, err)

	// Let A commit, releasing the lock.
	close(releaseHold)
	wg.Wait()
	require.NoError(t, holderErr)

	// Transaction C: lock is now free, so a try on the original key succeeds.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		acquired, tryErr := tx.TryAdvisoryLock(ctx, key)
		if tryErr != nil {
			return tryErr
		}
		assert.True(t, acquired, "expected lock to be free after holder committed")
		return nil
	})
	require.NoError(t, err)
}
