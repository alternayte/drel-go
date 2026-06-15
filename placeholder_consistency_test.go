package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/require"
)

// usesQuestionOnlyEngine builds an in-memory SQLite engine and a table; the raw
// API must accept $N placeholders on every path by rewriting them to ?.
func usesQuestionOnlyEngine(t *testing.T) *drel.Engine {
	t.Helper()
	e, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { e.Close() })
	_, err = e.Exec(context.Background(),
		"CREATE TABLE kv (id INTEGER PRIMARY KEY, v TEXT NOT NULL)")
	require.NoError(t, err)
	return e
}

func TestEngineRawPaths_RewriteDollarPlaceholders(t *testing.T) {
	e := usesQuestionOnlyEngine(t)
	ctx := context.Background()

	// Exec with $N
	n, err := e.Exec(ctx, "INSERT INTO kv (id, v) VALUES ($1, $2)", 1, "one")
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	// QueryRow with $N
	var v string
	require.NoError(t, e.QueryRow(ctx, "SELECT v FROM kv WHERE id = $1", 1).Scan(&v))
	require.Equal(t, "one", v)

	// Query with $N
	rows, err := e.Query(ctx, "SELECT v FROM kv WHERE id >= $1 ORDER BY id", 1)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var got string
	require.NoError(t, rows.Scan(&got))
	require.Equal(t, "one", got)
	require.NoError(t, rows.Err())
}

func TestTxRawPaths_RewriteDollarPlaceholders(t *testing.T) {
	e := usesQuestionOnlyEngine(t)
	ctx := context.Background()

	err := e.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO kv (id, v) VALUES ($1, $2)", 2, "two"); err != nil {
			return err
		}
		var v string
		if err := tx.QueryRow(ctx, "SELECT v FROM kv WHERE id = $1", 2).Scan(&v); err != nil {
			return err
		}
		require.Equal(t, "two", v)

		rows, err := tx.Query(ctx, "SELECT v FROM kv WHERE id = $1", 2)
		if err != nil {
			return err
		}
		defer rows.Close()
		require.True(t, rows.Next())
		var got string
		require.NoError(t, rows.Scan(&got))
		require.Equal(t, "two", got)
		return rows.Err()
	})
	require.NoError(t, err)
}
