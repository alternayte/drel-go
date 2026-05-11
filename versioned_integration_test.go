//go:build integration

package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupVersionedTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `
		CREATE TABLE v_products (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			price      INTEGER NOT NULL,
			version    INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)
	return engine
}

func TestIntegration_Versioned_UpdateIncrementsVersion(t *testing.T) {
	engine := setupVersionedTestDB(t)
	ctx := context.Background()

	// Insert a versioned product via a transaction.
	product := testmodels.NewVersionedProduct("Widget", 1000)
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		repo.Add(product)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, product.Version(), "initial version after insert should be 1")

	// Update the product name in a second transaction.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		p, err := repo.Find(ctx, product.ID())
		if err != nil {
			return err
		}
		p.SetName("UpdatedWidget")
		return nil
	})
	require.NoError(t, err)

	// Verify version was incremented in the database.
	row := engine.QueryRow(ctx, "SELECT version, name FROM v_products WHERE id = $1", product.ID())
	var dbVersion int
	var dbName string
	require.NoError(t, row.Scan(&dbVersion, &dbName))
	assert.Equal(t, 2, dbVersion, "version should be 2 after one update")
	assert.Equal(t, "UpdatedWidget", dbName)
}

func TestIntegration_Versioned_ConcurrencyConflict(t *testing.T) {
	engine := setupVersionedTestDB(t)
	ctx := context.Background()

	// Insert a versioned product.
	product := testmodels.NewVersionedProduct("Widget", 1000)
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		repo.Add(product)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, product.Version())

	// Start a new transaction: load the product (version=1), then
	// externally bump the version behind the transaction's back.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		p, err := repo.Find(ctx, product.ID())
		if err != nil {
			return err
		}

		// Simulate another transaction updating the row externally.
		_, err = engine.Exec(ctx,
			"UPDATE v_products SET version = 2, name = 'ExternalUpdate' WHERE id = $1",
			product.ID(),
		)
		if err != nil {
			return err
		}

		// Now modify the entity and let the transaction try to commit.
		// The WHERE version=1 clause should fail because version is now 2.
		p.SetName("StaleUpdate")
		return nil
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, drel.ErrConcurrencyConflict)

	// Verify the external update's value is still in the database (not overwritten).
	row := engine.QueryRow(ctx, "SELECT version, name FROM v_products WHERE id = $1", product.ID())
	var dbVersion int
	var dbName string
	require.NoError(t, row.Scan(&dbVersion, &dbName))
	assert.Equal(t, 2, dbVersion, "version should remain 2 from the external update")
	assert.Equal(t, "ExternalUpdate", dbName, "external update name should persist")
}
