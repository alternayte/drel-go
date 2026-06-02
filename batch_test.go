package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatch_SequentialFallback(t *testing.T) {
	engine, repo := setupPageRows(t, 7)
	ctx := context.Background()

	b := engine.NewBatch()
	all := drel.BatchAll(b, repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()))
	cnt := drel.BatchCount(b, repo.Where(drel.NewOrderedCol[int]("rank").Eq(0)))
	first := drel.BatchFirst(b, repo.OrderBy(drel.NewOrderedCol[int]("id").Asc()))
	missing := drel.BatchFirst(b, repo.Where(drel.NewOrderedCol[int]("id").Eq(9999)))

	require.NoError(t, b.Execute(ctx))

	items, err := all.Result()
	require.NoError(t, err)
	assert.Len(t, items, 7)

	n, err := cnt.Result()
	require.NoError(t, err)
	// rank = i%3 == 0 for i in 3,6 → 2 rows.
	assert.Equal(t, 2, n)

	f, err := first.Result()
	require.NoError(t, err)
	assert.Equal(t, 1, f.ID)

	_, err = missing.Result()
	assert.ErrorIs(t, err, drel.ErrNotFound)
}

func TestBatch_Empty(t *testing.T) {
	engine, _ := setupPageRows(t, 0)
	require.NoError(t, engine.NewBatch().Execute(context.Background()))
}
