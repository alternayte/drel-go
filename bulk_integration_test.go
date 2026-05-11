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

func TestIntegration_BulkInsert_SingleRow(t *testing.T) {
	engine := setupTestDB(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products := []*testmodels.Product{
		{Name: "Solo", Price: 100, InStock: true},
	}

	affected, err := repo.BulkInsert(ctx, products)
	require.NoError(t, err)
	assert.Equal(t, 1, affected)

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestIntegration_BulkInsert_ZeroRows(t *testing.T) {
	engine := setupTestDB(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	affected, err := repo.BulkInsert(ctx, []*testmodels.Product{})
	require.NoError(t, err)
	assert.Equal(t, 0, affected)

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestIntegration_BulkInsert_ManyRows(t *testing.T) {
	engine := setupTestDB(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products := make([]*testmodels.Product, 50)
	for i := range products {
		products[i] = &testmodels.Product{
			Name:    "Product" + string(rune('A'+i%26)),
			Price:   100 * (i + 1),
			InStock: i%2 == 0,
		}
	}

	affected, err := repo.BulkInsert(ctx, products)
	require.NoError(t, err)
	assert.Equal(t, 50, affected)

	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 50, count)
}

func TestIntegration_BulkUpdate_WithCondition(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// Doohickey (in_stock=false) and Whatchamacallit (in_stock=false) should be updated
	affected, err := repo.Where(testmodels.Products.InStock.IsFalse()).
		BulkUpdate(ctx, drel.Set(testmodels.Products.Price, 999))
	require.NoError(t, err)
	assert.Equal(t, 2, affected)

	// Verify the out-of-stock products now have price 999
	updated, err := repo.Where(testmodels.Products.Price.Eq(999)).All(ctx)
	require.NoError(t, err)
	assert.Len(t, updated, 2)

	// Verify in-stock products are unchanged
	inStock, err := repo.Where(testmodels.Products.InStock.IsTrue()).All(ctx)
	require.NoError(t, err)
	for _, p := range inStock {
		assert.NotEqual(t, 999, p.Price)
	}
}

func TestIntegration_BulkDelete_WithCondition(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// Delete out-of-stock products (Doohickey, Whatchamacallit)
	affected, err := repo.Where(testmodels.Products.InStock.IsFalse()).
		BulkDelete(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, affected)

	// Only in-stock products remain
	remaining, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, remaining)
}

func TestIntegration_BulkDelete_SoftDeleteModel(t *testing.T) {
	engine := setupSoftDeleteTestDB(t)
	ctx := context.Background()

	// Insert two sd_products via raw SQL
	_, err := engine.Exec(ctx, "INSERT INTO sd_products (name, price) VALUES ('A', 100)")
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO sd_products (name, price) VALUES ('B', 200)")
	require.NoError(t, err)

	repo := drel.NewRepository(engine, testmodels.SoftDeleteProductMeta)

	// Bulk delete where name='A' — should soft delete
	affected, err := repo.Where(testmodels.SoftDeleteProducts.Name.Eq("A")).
		BulkDelete(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, affected)

	// Verify deleted_at IS NOT NULL for product A
	row := engine.QueryRow(ctx, "SELECT deleted_at IS NOT NULL FROM sd_products WHERE name = 'A'")
	var hasDeletedAt bool
	require.NoError(t, row.Scan(&hasDeletedAt))
	assert.True(t, hasDeletedAt, "deleted_at should be set for soft-deleted product")

	// Verify product B is untouched
	row = engine.QueryRow(ctx, "SELECT deleted_at IS NULL FROM sd_products WHERE name = 'B'")
	var bIsNull bool
	require.NoError(t, row.Scan(&bIsNull))
	assert.True(t, bIsNull, "product B should not have deleted_at set")

	// Auto-filter should exclude soft-deleted product A
	products, err := repo.All(ctx)
	require.NoError(t, err)
	require.Len(t, products, 1)
	assert.Equal(t, "B", products[0].Name())
}
