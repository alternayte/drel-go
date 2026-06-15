package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Select/Aggregate/GroupBy must accept a *TxQueryBuilder and run inside the tx,
// observing the transaction's own uncommitted writes.
func TestSelectTx_ProjectionInsideTransaction(t *testing.T) {
	engine, _ := setupSelectEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		// New row visible only inside the tx.
		if _, err := tx.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Widget Z", "electronics", 99.0); err != nil {
			return err
		}
		repo := drel.NewTxRepository(tx, selectProductMeta)
		qb := repo.Where(drel.NewStringCol("category").Eq("electronics")).OrderBy(drel.NewStringCol("name").Asc())

		results, err := drel.Select[nameOnlyDTO](ctx, qb, drel.ColRef("name"))
		if err != nil {
			return err
		}
		// Widget A, Widget B (seeded) + Widget Z (uncommitted) = 3.
		require.Len(t, results, 3)
		assert.Equal(t, "Widget A", results[0].Name)
		assert.Equal(t, "Widget Z", results[2].Name)
		return nil
	})
	require.NoError(t, err)
}

func TestAggregateTx_SeesUncommittedWrite(t *testing.T) {
	engine, _ := setupSelectEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO products (name, category, price) VALUES (?, ?, ?)", "Widget Z", "electronics", 0.02); err != nil {
			return err
		}
		repo := drel.NewTxRepository(tx, selectProductMeta)
		qb := repo.Where(drel.NewStringCol("category").Eq("electronics"))
		total, err := drel.Aggregate[float64](ctx, qb, drel.Sum(drel.ColRef("price")))
		if err != nil {
			return err
		}
		// 9.99 + 19.99 + 0.02 = 30.00
		assert.InDelta(t, 30.00, total, 0.001)
		return nil
	})
	require.NoError(t, err)
}

func TestGroupByTx_InsideTransaction(t *testing.T) {
	engine, _ := setupSelectEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, selectProductMeta)
		qb := repo.OrderBy(drel.NewStringCol("category").Asc())
		results, err := drel.GroupBy[categoryStatsDTO](ctx, qb,
			[]drel.GroupSpec{drel.Group(drel.ColRef("category"))},
			[]drel.AliasedAgg{drel.As("total_price", drel.Sum(drel.ColRef("price")))},
		)
		if err != nil {
			return err
		}
		require.Len(t, results, 2)
		assert.Equal(t, "accessories", results[0].Category)
		assert.InDelta(t, 20.98, results[0].TotalPrice, 0.001)
		assert.Equal(t, "electronics", results[1].Category)
		assert.InDelta(t, 29.98, results[1].TotalPrice, 0.001)
		return nil
	})
	require.NoError(t, err)
}

// Engine-builder Select still works after the signature change (regression).
func TestSelect_EngineBuilderStillWorks(t *testing.T) {
	_, repo := setupSelectEngine(t)
	ctx := context.Background()
	qb := repo.OrderBy(drel.NewStringCol("name").Asc())
	results, err := drel.Select[nameOnlyDTO](ctx, qb, drel.ColRef("name"))
	require.NoError(t, err)
	require.Len(t, results, 4)
}
