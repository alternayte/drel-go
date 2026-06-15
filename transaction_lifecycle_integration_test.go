//go:build integration

package drel_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WithIsolation must actually begin the transaction at the requested level.
func TestIntegration_WithIsolation_SetsLevel(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()

	cases := []struct {
		level drel.IsolationLevel
		want  string // pg transaction_isolation text
	}{
		{drel.ReadCommitted, "read committed"},
		{drel.RepeatableRead, "repeatable read"},
		{drel.Serializable, "serializable"},
	}
	for _, tc := range cases {
		var got string
		err := engine.Transaction(ctx, func(tx *drel.Tx) error {
			return tx.QueryRow(ctx, "SHOW transaction_isolation").Scan(&got)
		}, drel.WithIsolation(tc.level))
		require.NoError(t, err)
		assert.Equal(t, tc.want, strings.ToLower(strings.TrimSpace(got)),
			"WithIsolation(%v) must set the level", tc.level)
	}
}

// WithReadOnly must make Postgres reject a write inside the transaction.
func TestIntegration_WithReadOnly_RejectsWrite(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		_, e := tx.Exec(ctx,
			"INSERT INTO products (name, price, in_stock) VALUES ($1, $2, $3)",
			"nope", 1, true)
		return e
	}, drel.WithReadOnly())
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "read-only",
		"a write inside a read-only tx must be rejected by Postgres")

	// And no row was written.
	n, err := drel.NewRepository(engine, testmodels.ProductMeta).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

// Savepoint release/rollback must work over the wire against real Postgres,
// including tracker restore on rollback (currently only proven on SQLite).
func TestIntegration_Savepoint_ReleaseAndRollback(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()
	repo := drel.NewRepository(engine, testmodels.ProductMeta)

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		r := drel.NewTxRepository(tx, testmodels.ProductMeta)
		r.Add(&testmodels.Product{Name: "kept", Price: 100, InStock: true})
		if e := tx.SaveChanges(ctx); e != nil {
			return e
		}
		// Released savepoint: its work commits with the outer tx.
		if e := tx.Savepoint(ctx, "ok", func(sp *drel.Tx) error {
			drel.NewTxRepository(sp, testmodels.ProductMeta).
				Add(&testmodels.Product{Name: "released", Price: 200, InStock: true})
			return sp.SaveChanges(ctx)
		}); e != nil {
			return e
		}
		// Rolled-back savepoint: its work is reverted (DB + tracker).
		spErr := tx.Savepoint(ctx, "bad", func(sp *drel.Tx) error {
			drel.NewTxRepository(sp, testmodels.ProductMeta).
				Add(&testmodels.Product{Name: "dropped", Price: 300, InStock: true})
			if e := sp.SaveChanges(ctx); e != nil {
				return e
			}
			return assert.AnError
		})
		require.Error(t, spErr)
		return nil // swallow so outer commits
	})
	require.NoError(t, err)

	n, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "kept + released survive; dropped is rolled back")

	exists, err := repo.Where(testmodels.Products.Name.Eq("dropped")).Exists(ctx)
	require.NoError(t, err)
	assert.False(t, exists)
}

// Two concurrent SERIALIZABLE transactions that read-then-write overlapping
// rows must produce a 40001 serialization failure on at least one, and that
// error must classify as drel.ErrSerializationFailure (including the
// commit-time case, which is the class real Postgres surfaces).
func TestIntegration_SerializableConflict_Classifies(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	ctx := context.Background()

	// Barrier so both txns read before either writes, forcing a conflict.
	var afterRead sync.WaitGroup
	afterRead.Add(2)

	bump := func() error {
		return engine.Transaction(ctx, func(tx *drel.Tx) error {
			var total int
			if e := tx.QueryRow(ctx, "SELECT COALESCE(SUM(price),0) FROM products").Scan(&total); e != nil {
				return e
			}
			afterRead.Done()
			afterRead.Wait() // both have read; now both write -> conflict
			_, e := tx.Exec(ctx,
				"INSERT INTO products (name, price, in_stock) VALUES ($1, $2, $3)",
				"sum", total, true)
			return e
		}, drel.WithIsolation(drel.Serializable))
	}

	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = bump() }()
	go func() { defer wg.Done(); errs[1] = bump() }()
	wg.Wait()

	serialized := false
	for _, e := range errs {
		if e != nil && assert.ErrorIs(t, e, drel.ErrSerializationFailure) {
			serialized = true
		}
	}
	require.True(t, serialized,
		"at least one concurrent SERIALIZABLE txn must fail with ErrSerializationFailure; got errs=%v", errs)
}
