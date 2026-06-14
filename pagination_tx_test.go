package drel_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTxPageRows(t *testing.T, n int) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()
	_, err = engine.Exec(ctx, `CREATE TABLE page_rows (
		id   INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		rank INTEGER NOT NULL
	)`)
	require.NoError(t, err)
	for i := 1; i <= n; i++ {
		_, err = engine.Exec(ctx, `INSERT INTO page_rows (name, rank) VALUES (?, ?)`,
			fmt.Sprintf("row-%02d", i), i%3)
		require.NoError(t, err)
	}
	return engine
}

func TestTxCursorPage_BackwardWalk(t *testing.T) {
	engine := setupTxPageRows(t, 20)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, pageRowMeta)
		mk := func() *drel.TxQueryBuilder[pageRow] {
			return repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()).Take(5)
		}
		p1, err := mk().Page(ctx)
		if err != nil {
			return err
		}
		p2, err := mk().After(p1.NextCursor).Page(ctx)
		if err != nil {
			return err
		}
		assert.True(t, p2.HasPrev)
		require.NotEmpty(t, p2.PreviousCursor)

		back, err := mk().Before(p2.PreviousCursor).Page(ctx)
		if err != nil {
			return err
		}
		gotIDs := make([]int, len(back.Items))
		for i, it := range back.Items {
			gotIDs[i] = it.ID
		}
		assert.Equal(t, []int{1, 2, 3, 4, 5}, gotIDs)
		return nil
	})
	require.NoError(t, err)
}
