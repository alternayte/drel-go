//go:build integration

package drel_test

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestIntegration_W3G1_HealthAndStats(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()

	require.NoError(t, engine.Ping(ctx))
	require.NoError(t, engine.HealthCheck(ctx))

	st := engine.Stats()
	assert.Greater(t, st.MaxConns, int32(0))
	assert.GreaterOrEqual(t, st.TotalConns, int32(0))
}

func TestIntegration_W3G1_PlaceholderConsistencyPostgres(t *testing.T) {
	engine := setupTestDB(t)
	seedProducts(t, engine)
	ctx := context.Background()

	// Engine.Query with native $N (Postgres rejects ? — proves no over-rewrite).
	rows, err := engine.Query(ctx, "SELECT name FROM products WHERE price > $1 ORDER BY name", 1000)
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, []string{"Gadget", "Thingamajig", "Whatchamacallit"}, names)

	// Engine.QueryRow with $N.
	var cnt int
	require.NoError(t, engine.QueryRow(ctx, "SELECT COUNT(*) FROM products WHERE price > $1", 1000).Scan(&cnt))
	assert.Equal(t, 3, cnt)

	// Tx.Query / Tx.Exec / Tx.QueryRow with $N.
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO products (name, price, in_stock) VALUES ($1, $2, $3)", "Sprocket", 999, true); err != nil {
			return err
		}
		r, err := tx.Query(ctx, "SELECT name FROM products WHERE name = $1", "Sprocket")
		if err != nil {
			return err
		}
		defer r.Close()
		require.True(t, r.Next())
		var n string
		require.NoError(t, r.Scan(&n))
		assert.Equal(t, "Sprocket", n)
		return r.Err()
	}))

	// RawQuery scalar against Postgres.
	ids, err := drel.RawQuery[int](ctx, engine, "SELECT id FROM products WHERE name = $1", "Sprocket")
	require.NoError(t, err)
	require.Len(t, ids, 1)
}

func TestIntegration_W3G1_QueryTimeout(t *testing.T) {
	engine := setupTestDB(t)
	ctx := context.Background()

	// A caller deadline shorter than the query duration aborts the query; this
	// proves the timeout plumbing reaches the driver. (The engine default path is
	// unit-tested in timeout_test.go.) The error may surface on Query() or on
	// rows.Next()/rows.Err() depending on the driver, so we check both paths.
	tctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	rows, queryErr := engine.Query(tctx, "SELECT pg_sleep($1)", 2)
	if queryErr != nil {
		// Error surfaced immediately — timeout fires before or during send.
		return
	}
	defer rows.Close()
	// Drain rows; the timeout error surfaces via rows.Err() when Next returns false.
	for rows.Next() {
	}
	iterErr := rows.Err()
	if iterErr == nil {
		t.Fatal("a short caller deadline must abort the slow query — no error on Query or iteration")
	}
}

// TestIntegration_W3G1_EngineDefaultTimeoutMultiRowSuccess proves that a
// generous engine-default WithQueryTimeout does NOT cancel a multi-row result
// set before the caller finishes draining it. This is the positive regression
// test for the bug where queryRoutedTimeout called defer cancel() and cancelled
// the context the instant it returned the Rows handle.
func TestIntegration_W3G1_EngineDefaultTimeoutMultiRowSuccess(t *testing.T) {
	ctx := context.Background()

	// Spin up a fresh Postgres container and connect with a 5s engine-default
	// timeout. We cannot reuse setupTestDB's engine because it does not set
	// WithQueryTimeout. Use generate_series so no table setup is required.
	container, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("dreltest"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	timedEngine, err := drel.NewEngine(connStr,
		drel.WithContext(ctx),
		drel.WithQueryTimeout(5*time.Second),
	)
	require.NoError(t, err)
	t.Cleanup(func() { timedEngine.Close() })

	// generate_series(1,500) yields 500 rows; the 5 s engine default must not
	// fire before the caller has had a chance to drain them all.
	rows, err := timedEngine.Query(ctx, "SELECT n FROM generate_series(1, 500) AS gs(n)")
	require.NoError(t, err, "engine-default timeout must not abort query start")
	defer rows.Close()

	count := 0
	for rows.Next() {
		var n int
		require.NoError(t, rows.Scan(&n))
		count++
	}
	require.NoError(t, rows.Err(),
		"rows.Err() must be nil after full drain — engine default timeout must not cancel before Close()")
	assert.Equal(t, 500, count, "all 500 rows must be returned")
}
