//go:build integration

package drel_test

import (
	"context"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
