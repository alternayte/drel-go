//go:build integration

package drel_test

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_TimeColumn_Between(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// All seeded rows were created moments ago; a wide window returns all 5.
	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)
	products, err := repo.Where(testmodels.Products.CreatedAt.Between(from, to)).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 5)

	// A window entirely in the past returns none.
	none, err := repo.Where(testmodels.Products.CreatedAt.Between(
		time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour),
	)).All(ctx)
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestIntegration_TimeColumn_GT_And_Before(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	gt, err := repo.Where(testmodels.Products.CreatedAt.GT(time.Now().Add(-1 * time.Hour))).All(ctx)
	require.NoError(t, err)
	assert.Len(t, gt, 5)

	before, err := repo.Where(testmodels.Products.CreatedAt.Before(time.Now().Add(-1 * time.Hour))).All(ctx)
	require.NoError(t, err)
	assert.Empty(t, before)
}

func TestIntegration_NotIn(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(testmodels.Products.Name.NotIn("Widget", "Gadget")).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 3)
	for _, p := range products {
		assert.NotEqual(t, "Widget", p.Name)
		assert.NotEqual(t, "Gadget", p.Name)
	}
}

func TestIntegration_Like(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(testmodels.Products.Name.Like("W%")).All(ctx)
	require.NoError(t, err)
	// "Widget" and "Whatchamacallit" start with capital W.
	assert.Len(t, products, 2)
}

func TestIntegration_ILike(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// lowercase pattern must still match capitalized names via ILIKE.
	products, err := repo.Where(testmodels.Products.Name.ILike("w%")).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 2)
}

func TestIntegration_Not(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(drel.Not(testmodels.Products.Name.Eq("Widget"))).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 4)
	for _, p := range products {
		assert.NotEqual(t, "Widget", p.Name)
	}
}
