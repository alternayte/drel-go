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

func setupSQLiteProducts(t *testing.T) *drel.Engine {
	t.Helper()
	ctx := context.Background()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	_, execErr := engine.Exec(ctx, `
		CREATE TABLE products (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			name       TEXT NOT NULL,
			price      INTEGER NOT NULL,
			in_stock   INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, execErr)

	rows := []struct {
		name    string
		price   int
		inStock int
	}{
		{"Widget", 1000, 1},
		{"Gadget", 2500, 1},
		{"Doohickey", 500, 0},
		{"Thingamajig", 1500, 1},
		{"Whatchamacallit", 3000, 0},
	}
	for _, r := range rows {
		_, insertErr := engine.Exec(ctx,
			"INSERT INTO products (name, price, in_stock) VALUES (?, ?, ?)",
			r.name, r.price, r.inStock,
		)
		require.NoError(t, insertErr)
	}
	return engine
}

func TestIntegration_SQLite_TimeColumn_Between(t *testing.T) {
	engine := setupSQLiteProducts(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	from := time.Now().Add(-1 * time.Hour)
	to := time.Now().Add(1 * time.Hour)
	products, err := repo.Where(testmodels.Products.CreatedAt.Between(from, to)).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 5)
}

func TestIntegration_SQLite_NotIn(t *testing.T) {
	engine := setupSQLiteProducts(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(testmodels.Products.Name.NotIn("Widget", "Gadget")).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 3)
}

func TestIntegration_SQLite_Like(t *testing.T) {
	engine := setupSQLiteProducts(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(testmodels.Products.Name.Like("W%")).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 2)
}

func TestIntegration_SQLite_ILike_MapsToLike(t *testing.T) {
	engine := setupSQLiteProducts(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	// SQLite LIKE is case-insensitive for ASCII; ILike must match capitalized names.
	products, err := repo.Where(testmodels.Products.Name.ILike("w%")).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 2)
}

func TestIntegration_SQLite_Not(t *testing.T) {
	engine := setupSQLiteProducts(t)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	products, err := repo.Where(drel.Not(testmodels.Products.Name.Eq("Widget"))).All(ctx)
	require.NoError(t, err)
	assert.Len(t, products, 4)
}
