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

// TestIntegration_Batch_Pipeline verifies that the pgx pipeline path returns
// correct typed results for several queries sent in one round-trip.
func TestIntegration_Batch_Pipeline(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	b := engine.NewBatch()
	all := drel.BatchAll(b, repo.OrderBy(testmodels.Products.ID.Asc()))
	inStock := drel.BatchCount(b, repo.Where(testmodels.Products.InStock.IsTrue()))
	first := drel.BatchFirst(b, repo.OrderBy(testmodels.Products.Price.Asc()))

	require.NoError(t, b.Execute(ctx))

	items, err := all.Result()
	require.NoError(t, err)
	assert.Len(t, items, 5)

	n, err := inStock.Result()
	require.NoError(t, err)
	assert.Equal(t, 3, n) // Widget, Gadget, Thingamajig

	cheapest, err := first.Result()
	require.NoError(t, err)
	assert.Equal(t, "Doohickey", cheapest.Name) // price 500
}
