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

// TestIntegration_Batch_PipelineClassifiesQueryError verifies two things on the
// real pgx driver: (1) a successful pipeline round-trip returns the correct
// typed results via Execute, and (2) a real unique-violation surfaced through
// engine.Exec classifies as ErrUniqueViolation. Classification of pipeline
// Query/SendBatch errors is unit-tested in batch_classification_test.go with a
// fake *pgconn.PgError; this integration test pins the "error is surfaced and
// classified, not dropped" behaviour on the real driver.
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

	// Drive a real unique violation through engine.Exec to confirm the
	// classification contract holds on the real pgx driver end-to-end.
	_, err = engine.Exec(ctx, `CREATE TABLE u (id SERIAL PRIMARY KEY, k TEXT UNIQUE)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO u (k) VALUES ('a')`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, `INSERT INTO u (k) VALUES ('a')`)
	require.True(t, errors.Is(err, drel.ErrUniqueViolation), "got %v", err)
}
