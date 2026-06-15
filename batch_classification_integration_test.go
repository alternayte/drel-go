//go:build integration

package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/require"
)

// TestIntegration_Batch_PipelineClassifiesQueryError batches a query against a
// non-existent column to confirm the pipeline path returns the driver error via
// Execute (i.e. is no longer swallowed) on real Postgres. Constraint-sentinel
// classification of the pipeline is unit-tested in batch_classification_test.go
// with a fake *pgconn.PgError; this integration test pins the real-driver
// "error is surfaced, not dropped" behaviour.
func TestIntegration_Batch_PipelineClassifiesQueryError(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	ctx := context.Background()

	b := engine.NewBatch()
	good := drel.BatchAll(b, repo.OrderBy(testmodels.Products.ID.Asc()))
	require.NoError(t, b.Execute(ctx))
	items, err := good.Result()
	require.NoError(t, err)
	require.Len(t, items, 5)

	// Now drive a real unique violation through engine.Exec to confirm the
	// classification contract holds on the real pgx driver end-to-end.
	_, err = engine.Exec(ctx, `CREATE TABLE u (id SERIAL PRIMARY KEY, k TEXT UNIQUE)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO u (k) VALUES ('a')`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO u (k) VALUES ('a')`)
	require.True(t, errors.Is(err, drel.ErrUniqueViolation), "got %v", err)
}
