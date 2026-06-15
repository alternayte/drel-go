package drel_test

import (
	"context"
	"errors"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedItems(t *testing.T, engine *drel.Engine, titles ...string) {
	t.Helper()
	ctx := context.Background()
	for _, ti := range titles {
		_, err := engine.Exec(ctx, "INSERT INTO items (title) VALUES (?)", ti)
		require.NoError(t, err)
	}
}

func TestTxBulkUpdate_RunsInsideTransaction(t *testing.T) {
	engine := setupSQLiteEngine(t)
	seedItems(t, engine, "a", "a", "b")
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		n, err := repo.Where(drel.NewStringCol("title").Eq("a")).
			BulkUpdate(ctx, drel.Set(drel.NewStringCol("title"), "z"))
		if err != nil {
			return err
		}
		assert.Equal(t, 2, n)
		return nil
	})
	require.NoError(t, err)

	repo := drel.NewRepository(engine, sqliteItemMeta)
	count, err := repo.Where(drel.NewStringCol("title").Eq("z")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestTxBulkDelete_RollsBackWithSurroundingWork(t *testing.T) {
	engine := setupSQLiteEngine(t)
	seedItems(t, engine, "a", "b", "c")
	ctx := context.Background()

	sentinel := errors.New("boom")
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		if _, err := repo.Where(drel.NewStringCol("title").Eq("a")).BulkDelete(ctx); err != nil {
			return err
		}
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)

	repo := drel.NewRepository(engine, sqliteItemMeta)
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "bulk delete must roll back with the surrounding transaction")
}

func TestTxBulkDelete_RequiresFilter(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		_, err := repo.AsNoTracking().BulkDelete(ctx)
		return err
	})
	require.ErrorIs(t, err, drel.ErrBulkDeleteRequiresFilter)
}
