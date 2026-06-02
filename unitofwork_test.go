package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitOfWork_AddSaveLoadMutate(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, sqliteItemMeta)

	// Stage two inserts, flush in one SaveChanges.
	a := &sqliteItem{Title: "A"}
	b := &sqliteItem{Title: "B"}
	repo.Add(a)
	repo.Add(b)
	require.NoError(t, uow.SaveChanges(ctx))
	assert.NotZero(t, a.ID)
	assert.NotZero(t, b.ID)

	// Tracked read + mutate + save (only changed columns persist).
	loaded, err := repo.All(ctx)
	require.NoError(t, err)
	require.Len(t, loaded, 2)
	loaded[0].Title = "A-updated"
	require.NoError(t, uow.SaveChanges(ctx))

	// Verify with a fresh untracked read.
	check := drel.NewRepository(engine, sqliteItemMeta)
	got, err := check.Find(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "A-updated", got.Title)
}

func TestUnitOfWork_Remove(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, sqliteItemMeta)
	item := &sqliteItem{Title: "doomed"}
	repo.Add(item)
	require.NoError(t, uow.SaveChanges(ctx))

	found, err := repo.Find(ctx, item.ID)
	require.NoError(t, err)
	require.NoError(t, repo.Remove(found))
	require.NoError(t, uow.SaveChanges(ctx))

	_, err = drel.NewRepository(engine, sqliteItemMeta).Find(ctx, item.ID)
	assert.ErrorIs(t, err, drel.ErrNotFound)
}

func TestUnitOfWork_AsNoTracking(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, sqliteItemMeta)
	repo.Add(&sqliteItem{Title: "orig"})
	require.NoError(t, uow.SaveChanges(ctx))

	// Untracked load — mutation must not be persisted.
	items, err := repo.AsNoTracking().All(ctx)
	require.NoError(t, err)
	require.Len(t, items, 1)
	items[0].Title = "should-not-persist"
	require.NoError(t, uow.SaveChanges(ctx))

	got, err := drel.NewRepository(engine, sqliteItemMeta).All(ctx)
	require.NoError(t, err)
	assert.Equal(t, "orig", got[0].Title)
}

func TestUnitOfWork_RollbackOnError(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	// A before-commit hook that fails should roll back the whole SaveChanges.
	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		return assert.AnError
	})

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, sqliteItemMeta)
	repo.Add(&sqliteItem{Title: "x"})
	err := uow.SaveChanges(ctx)
	require.Error(t, err)

	n, err := drel.NewRepository(engine, sqliteItemMeta).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "failed SaveChanges must roll back")
}
