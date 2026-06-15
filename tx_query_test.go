package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxQuery_MultiRowRawSelect(t *testing.T) {
	engine, _ := setupPageRows(t, 5)
	ctx := context.Background()

	var names []string
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		rows, err := tx.Query(ctx, "SELECT name FROM page_rows ORDER BY id ASC")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				return err
			}
			names = append(names, n)
		}
		return rows.Err()
	})
	require.NoError(t, err)
	require.Len(t, names, 5)
	assert.Equal(t, "row-01", names[0])
	assert.Equal(t, "row-05", names[4])
}

func TestTxQuery_SeesUncommittedWrites(t *testing.T) {
	engine, _ := setupPageRows(t, 0)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO page_rows (name, rank) VALUES (?, ?)", "in-tx", 1); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "SELECT name FROM page_rows WHERE rank = ?", 1)
		if err != nil {
			return err
		}
		defer rows.Close()
		require.True(t, rows.Next(), "tx.Query must see the uncommitted insert")
		var n string
		require.NoError(t, rows.Scan(&n))
		assert.Equal(t, "in-tx", n)
		return rows.Err()
	})
	require.NoError(t, err)
}
