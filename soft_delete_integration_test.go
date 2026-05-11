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

func setupSoftDeleteTestDB(t *testing.T) *drel.Engine {
	t.Helper()
	engine := setupTestDB(t)
	ctx := context.Background()
	_, err := engine.Exec(ctx, `
		CREATE TABLE sd_products (
			id         SERIAL PRIMARY KEY,
			name       TEXT NOT NULL,
			price      INTEGER NOT NULL,
			deleted_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	require.NoError(t, err)
	return engine
}

func TestIntegration_SoftDelete_RemoveSetsDeletedAt(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert a product via transaction
	var productID int
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.SoftDeleteProductMeta)
		p := testmodels.NewSoftDeleteProduct("Widget", 1000)
		repo.Add(p)
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		productID = p.ID()

		// Now soft-delete it
		return repo.Remove(p)
	})
	require.NoError(t, err)

	// Verify deleted_at is set via raw SQL
	row := engine.QueryRow(ctx,
		"SELECT deleted_at IS NOT NULL FROM sd_products WHERE id = $1", productID)
	var hasDeletedAt bool
	require.NoError(t, row.Scan(&hasDeletedAt))
	assert.True(t, hasDeletedAt, "deleted_at should be set after soft delete")
}

func TestIntegration_SoftDelete_AutoFilter(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert two products, soft-delete one
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.SoftDeleteProductMeta)

		p1 := testmodels.NewSoftDeleteProduct("Visible", 500)
		repo.Add(p1)

		p2 := testmodels.NewSoftDeleteProduct("Deleted", 700)
		repo.Add(p2)

		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}

		return repo.Remove(p2)
	})
	require.NoError(t, err)

	// Query via read-only repository — soft-deleted product should be filtered out
	repo := drel.NewRepository(engine, testmodels.SoftDeleteProductMeta)
	products, err := repo.All(ctx)
	require.NoError(t, err)
	require.Len(t, products, 1)
	assert.Equal(t, "Visible", products[0].Name())
}

func TestIntegration_SoftDelete_Unscoped(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert two products, soft-delete one
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.SoftDeleteProductMeta)

		p1 := testmodels.NewSoftDeleteProduct("Visible", 500)
		repo.Add(p1)

		p2 := testmodels.NewSoftDeleteProduct("Deleted", 700)
		repo.Add(p2)

		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}

		return repo.Remove(p2)
	})
	require.NoError(t, err)

	// Unscoped should return all products including soft-deleted
	repo := drel.NewRepository(engine, testmodels.SoftDeleteProductMeta)
	products, err := repo.Unscoped().All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 2)
}

func TestIntegration_SoftDelete_HardRemove(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert a product and hard-delete it
	var productID int
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.SoftDeleteProductMeta)

		p := testmodels.NewSoftDeleteProduct("ToBeGone", 999)
		repo.Add(p)

		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		productID = p.ID()

		return tx.HardRemove(p)
	})
	require.NoError(t, err)

	// Row should be completely gone — even unscoped won't find it
	row := engine.QueryRow(ctx,
		"SELECT COUNT(*) FROM sd_products WHERE id = $1", productID)
	var count int
	require.NoError(t, row.Scan(&count))
	assert.Equal(t, 0, count, "hard-deleted row should not exist in DB at all")
}

func TestIntegration_SoftDelete_CountExcludesDeleted(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert two products, soft-delete one
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, testmodels.SoftDeleteProductMeta)

		p1 := testmodels.NewSoftDeleteProduct("Kept", 100)
		repo.Add(p1)

		p2 := testmodels.NewSoftDeleteProduct("Removed", 200)
		repo.Add(p2)

		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}

		return repo.Remove(p2)
	})
	require.NoError(t, err)

	// Count should only reflect non-deleted products
	repo := drel.NewRepository(engine, testmodels.SoftDeleteProductMeta)
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "Count should exclude soft-deleted products")
}
