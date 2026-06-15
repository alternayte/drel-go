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

func TestSavepoint_NestedSameName(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	// Reusing the same savepoint name at different nesting levels must not
	// collide: the inner rollback drops only "c", the outer keeps "a"+"b".
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, sqliteItemMeta).Add(&sqliteItem{Title: "a"})
		if err := tx.SaveChanges(ctx); err != nil {
			return err
		}
		return tx.Savepoint(ctx, "sp", func(sp1 *drel.Tx) error {
			drel.NewTxRepository(sp1, sqliteItemMeta).Add(&sqliteItem{Title: "b"})
			if err := sp1.SaveChanges(ctx); err != nil {
				return err
			}
			_ = sp1.Savepoint(ctx, "sp", func(sp2 *drel.Tx) error { // same name
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
	assert.Equal(t, 2, countItems(t, engine))
}

// On the SUCCESS path, if RELEASE SAVEPOINT fails the savepoint's staged adds
// must be reverted from the tracker (mirroring the rollback branch) so the
// outer commit does not re-flush them. We provoke a deterministic RELEASE
// failure by pre-releasing the framework's savepoint inside fn: the first
// savepoint named "rel" in a fresh tx is emitted as "sp_rel_1"
// (sanitizeSavepoint prefixes "sp_", then the per-tx counter appends "_1").
func TestSavepoint_ReleaseFailureSuccessPath_RevertsTracker(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		drel.NewTxRepository(tx, sqliteItemMeta).Add(&sqliteItem{Title: "outer"})
		if e := tx.SaveChanges(ctx); e != nil {
			return e
		}

		spErr := tx.Savepoint(ctx, "rel", func(sp *drel.Tx) error {
			// Stage an add but do NOT flush it inside the savepoint, so it is
			// still pending in the tracker when RELEASE runs.
			drel.NewTxRepository(sp, sqliteItemMeta).Add(&sqliteItem{Title: "inner-staged"})
			// Pre-release the framework's savepoint so the framework's own
			// RELEASE on the success path fails ("no such savepoint").
			_, _ = sp.Exec(ctx, "RELEASE SAVEPOINT sp_rel_1")
			return nil // success path -> framework RELEASE now fails
		})
		// The RELEASE failure must surface as an error.
		require.Error(t, spErr)
		require.Contains(t, spErr.Error(), "release savepoint")
		// Swallow it so the outer transaction still commits; the staged inner
		// add must have been reverted and therefore NOT flushed.
		return nil
	})
	require.NoError(t, err)

	// Only "outer" must survive. Before the fix, "inner-staged" leaks into the
	// outer commit (count == 2).
	assert.Equal(t, 1, countItems(t, engine))
}

func TestAttach_UnchangedInsertOnlyMetaDoesNotPanic(t *testing.T) {
	// evItemMeta (defined in outbox_test.go) has no Snapshot/Diff. Attaching as
	// StateUnchanged must not panic even though such a model can't be diffed.
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()
	ctx := context.Background()
	_, err = engine.Exec(ctx, `CREATE TABLE ev_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		_ = engine.Transaction(ctx, func(tx *drel.Tx) error {
			repo := drel.NewTxRepository(tx, evItemMeta)
			repo.Attach(&evItem{ID: 1, Name: "x"}, drel.StateUnchanged)
			return tx.SaveChanges(ctx)
		})
	})
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
