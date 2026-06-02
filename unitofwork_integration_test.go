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

// TestIntegration_UnitOfWork exercises the UnitOfWork add/track/update/remove
// cycle against real Postgres, including read-your-writes across SaveChanges.
func TestIntegration_UnitOfWork(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, testmodels.ProductMeta)

	p := &testmodels.Product{Name: "Widget", Price: 100, InStock: true}
	repo.Add(p)
	require.NoError(t, uow.SaveChanges(ctx))
	require.NotZero(t, p.ID)

	// Read-your-writes: the same uow sees the committed row, tracked.
	loaded, err := repo.Find(ctx, p.ID)
	require.NoError(t, err)
	loaded.Price = 250
	require.NoError(t, uow.SaveChanges(ctx))

	// Verify the partial UPDATE persisted via an untracked read.
	check := drel.NewRepository(engine, testmodels.ProductMeta)
	got, err := check.Find(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 250, got.Price)

	// Remove + save.
	require.NoError(t, repo.Remove(loaded))
	require.NoError(t, uow.SaveChanges(ctx))
	_, err = check.Find(ctx, p.ID)
	assert.ErrorIs(t, err, drel.ErrNotFound)
}
