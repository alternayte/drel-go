//go:build integration

package drel_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/internal/testmodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestIntegration_DevMode_MissingIndex verifies that a slow SELECT performing a
// sequential scan on real Postgres triggers the missing-index hint.
func TestIntegration_DevMode_MissingIndex(t *testing.T) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("dreltest"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// A 1ns threshold marks every query "slow" so the EXPLAIN probe always runs;
	// the WHERE on the unindexed `name` column forces a sequential scan.
	engine, err := drel.NewEngine(dsn, drel.WithContext(ctx),
		drel.WithDevMode(), drel.WithLogger(logger), drel.WithSlowQueryThreshold(time.Nanosecond))
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	_, err = engine.Exec(ctx, `CREATE TABLE products (
		id SERIAL PRIMARY KEY, name TEXT NOT NULL, price INTEGER NOT NULL,
		in_stock BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, "INSERT INTO products (name, price) VALUES ('Widget', 10), ('Gadget', 20)")
	require.NoError(t, err)

	repo := drel.NewRepository(engine, testmodels.ProductMeta)
	_, err = repo.Where(testmodels.Products.Name.Eq("Widget")).All(ctx)
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "sequential scan")
}
