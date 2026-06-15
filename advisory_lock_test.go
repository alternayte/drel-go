package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// On SQLite, advisory locks are a documented no-op: AdvisoryLock returns nil and
// TryAdvisoryLock returns (true, nil). This proves the runtime wiring and the
// unsupported-dialect branch without needing a real Postgres instance.
func TestAdvisoryLock_SQLiteNoOp(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()

	ctx := context.Background()
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		if lockErr := tx.AdvisoryLock(ctx, 12345); lockErr != nil {
			return lockErr
		}
		acquired, tryErr := tx.TryAdvisoryLock(ctx, 12345)
		if tryErr != nil {
			return tryErr
		}
		assert.True(t, acquired, "SQLite TryAdvisoryLock must report acquired")
		return nil
	})
	require.NoError(t, err)
}
