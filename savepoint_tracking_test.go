package drel_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertItem(t *testing.T, engine *drel.Engine, title string) int {
	t.Helper()
	ctx := context.Background()
	item := &sqliteItem{Title: title}
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, sqliteItemMeta).Add(item)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)
	return item.ID
}

func countItems(t *testing.T, engine *drel.Engine) int {
	t.Helper()
	n, err := drel.NewRepository(engine, sqliteItemMeta).Count(context.Background())
	require.NoError(t, err)
	return n
}

// ─── Savepoints ──────────────────────────────────────────────────────────────

func TestSavepoint_ReleaseCommits(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(&sqliteItem{Title: "outer"})
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		return tx.Savepoint(ctx, "sp1", func(sp *drel.Tx) error {
			drel.NewTxRepository(sp, sqliteItemMeta).Add(&sqliteItem{Title: "inner"})
			return sp.SaveChanges(ctx)
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 2, countItems(t, engine))
}

func TestSavepoint_RollbackUndoesInnerButKeepsOuter(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(&sqliteItem{Title: "outer"})
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		// The savepoint's work is rolled back; the error is intentionally swallowed
		// so the outer transaction still commits.
		spErr := tx.Savepoint(ctx, "risky", func(sp *drel.Tx) error {
			drel.NewTxRepository(sp, sqliteItemMeta).Add(&sqliteItem{Title: "inner"})
			if err := sp.SaveChanges(ctx); err != nil {
				return err
			}
			return fmt.Errorf("boom")
		})
		assert.Error(t, spErr)
		return nil
	})
	require.NoError(t, err)
	// Only "outer" survives — the inner insert was rolled back AND the tracker was
	// reverted so it was not re-flushed on the outer commit.
	assert.Equal(t, 1, countItems(t, engine))
}

func TestSavepoint_Nested(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, sqliteItemMeta).Add(&sqliteItem{Title: "a"})
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		return tx.Savepoint(ctx, "outer_sp", func(sp1 *drel.Tx) error {
			drel.NewTxRepository(sp1, sqliteItemMeta).Add(&sqliteItem{Title: "b"})
			if err := sp1.SaveChanges(ctx); err != nil {
				return err
			}
			_ = sp1.Savepoint(ctx, "inner_sp", func(sp2 *drel.Tx) error {
				drel.NewTxRepository(sp2, sqliteItemMeta).Add(&sqliteItem{Title: "c"})
				if err := sp2.SaveChanges(ctx); err != nil {
					return err
				}
				return fmt.Errorf("drop c")
			})
			return nil
		})
	})
	require.NoError(t, err)
	assert.Equal(t, 2, countItems(t, engine)) // a and b, not c
}

// ─── Tracking by default vs AsNoTracking ─────────────────────────────────────

func TestTxQuery_TracksByDefault(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()
	insertItem(t, engine, "Original")

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		items, err := repo.Where(drel.NewStringCol("title").Eq("Original")).All(ctx)
		if err != nil {
			return err
		}
		require.Len(t, items, 1)
		items[0].Title = "Tracked" // mutation must be detected on commit
		return nil
	})
	require.NoError(t, err)

	found, err := drel.NewRepository(engine, sqliteItemMeta).Where(drel.NewStringCol("title").Eq("Tracked")).First(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Tracked", found.Title)
}

func TestTxQuery_AsNoTrackingDoesNotPersist(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()
	insertItem(t, engine, "Original")

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		items, err := repo.AsNoTracking().Where(drel.NewStringCol("title").Eq("Original")).All(ctx)
		if err != nil {
			return err
		}
		require.Len(t, items, 1)
		items[0].Title = "ShouldNotPersist"
		return nil
	})
	require.NoError(t, err)

	n, err := drel.NewRepository(engine, sqliteItemMeta).Where(drel.NewStringCol("title").Eq("Original")).Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "untracked mutation must not be persisted")
}

// ─── Attach / Detach ─────────────────────────────────────────────────────────

func TestAttach_ModifiedPersistsFullUpdate(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()
	id := insertItem(t, engine, "Original")

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		// Entity built outside the tracker (e.g. from a request body).
		ext := &sqliteItem{ID: id, Title: "Attached"}
		repo.Attach(ext, drel.StateModified)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	found, err := drel.NewRepository(engine, sqliteItemMeta).Find(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Attached", found.Title)
}

func TestDetach_StopsTracking(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()
	id := insertItem(t, engine, "Original")

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		found, err := repo.Find(ctx, id)
		if err != nil {
			return err
		}
		repo.Detach(found)
		found.Title = "ShouldNotPersist"
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	found, err := drel.NewRepository(engine, sqliteItemMeta).Find(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "Original", found.Title)
}
