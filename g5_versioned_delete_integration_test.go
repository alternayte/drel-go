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

func TestIntegration_VersionedDelete_StaleVersionConflicts(t *testing.T) {
	engine := setupVersionedTestDB(t)
	ctx := context.Background()

	product := testmodels.NewVersionedProduct("Widget", 1000)
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, testmodels.VersionedProductMeta).Add(product)
		return nil
	}))
	require.Equal(t, 1, product.Version())

	// Load at version 1, externally bump to 2, then attempt the stale delete.
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		p, err := repo.Find(ctx, product.ID())
		if err != nil {
			return err
		}
		if _, err := engine.Exec(ctx,
			"UPDATE v_products SET version = 2 WHERE id = $1", product.ID()); err != nil {
			return err
		}
		return repo.Remove(p)
	})
	require.ErrorIs(t, err, drel.ErrConcurrencyConflict)

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM v_products WHERE id = $1", product.ID())
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 1, count, "stale-version delete must not remove the Postgres row")
}

func TestIntegration_VersionedDelete_CurrentVersionSucceeds(t *testing.T) {
	engine := setupVersionedTestDB(t)
	ctx := context.Background()

	product := testmodels.NewVersionedProduct("Widget", 1000)
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, testmodels.VersionedProductMeta).Add(product)
		return nil
	}))

	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.VersionedProductMeta)
		p, err := repo.Find(ctx, product.ID())
		if err != nil {
			return err
		}
		return repo.Remove(p)
	}))

	row := engine.QueryRow(ctx, "SELECT COUNT(*) FROM v_products WHERE id = $1", product.ID())
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count, "current-version delete must remove the Postgres row")
}
