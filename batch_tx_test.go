package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxBatch_RunsOnTransactionConnection(t *testing.T) {
	engine, _ := setupPageRows(t, 5)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		// Insert a 6th row inside the tx; the batch must see it.
		if _, err := tx.Exec(ctx, "INSERT INTO page_rows (name, rank) VALUES (?, ?)", "row-06", 0); err != nil {
			return err
		}
		repo := drel.NewRepository(engine, pageRowMeta)

		b := tx.NewBatch()
		all := drel.BatchAll(b, repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()))
		cnt := drel.BatchCount(b, repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()))
		if err := b.Execute(ctx); err != nil {
			return err
		}

		items, err := all.Result()
		if err != nil {
			return err
		}
		require.Len(t, items, 6, "batch on tx must see the uncommitted insert")
		assert.Equal(t, "row-06", items[5].Name)

		n, err := cnt.Result()
		if err != nil {
			return err
		}
		assert.Equal(t, 6, n)
		return nil
	})
	require.NoError(t, err)
}

func TestTxBatch_Empty(t *testing.T) {
	engine, _ := setupPageRows(t, 0)
	ctx := context.Background()
	require.NoError(t, engine.Transaction(ctx, func(tx *drel.Tx) error {
		return tx.NewBatch().Execute(ctx)
	}))
}
