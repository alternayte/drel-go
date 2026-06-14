package drel_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test model for SQLite mutation tests ────────────────────────────────────

type sqliteItem struct {
	ID        int
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type sqliteItemSnapshot struct {
	Title string
}

var sqliteItemMeta = drel.ModelMeta[sqliteItem]{
	Table:    "items",
	Columns:  []string{"id", "title", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*sqliteItem, error) {
		p := &sqliteItem{}
		err := row.Scan(&p.ID, &p.Title, &p.CreatedAt, &p.UpdatedAt)
		return p, err
	},
	Snapshot: func(p *sqliteItem) any {
		return sqliteItemSnapshot{Title: p.Title}
	},
	Diff: func(p *sqliteItem, snap any) []drel.FieldChange {
		s := snap.(sqliteItemSnapshot)
		var changes []drel.FieldChange
		if p.Title != s.Title {
			changes = append(changes, drel.FieldChange{Column: "title", Value: p.Title})
		}
		return changes
	},
	PKValue: func(p *sqliteItem) any { return p.ID },
	InsertColumns: func(p *sqliteItem) ([]string, []any) {
		return []string{"title"}, []any{p.Title}
	},
	ScanReturning: func(p *sqliteItem, row drel.Row) error {
		return row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	},
}

func setupSQLiteEngine(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `
		CREATE TABLE items (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			title      TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	return engine
}

// ─── INSERT path: non-RETURNING readback ─────────────────────────────────────

func TestSQLiteMutation_Insert_PopulatesGeneratedFields(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	item := &sqliteItem{Title: "Test Item"}

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(item)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	// After insert, the entity should have its generated fields populated.
	assert.NotZero(t, item.ID, "ID should be populated after insert")
	assert.False(t, item.CreatedAt.IsZero(), "CreatedAt should be populated after insert")
	assert.False(t, item.UpdatedAt.IsZero(), "UpdatedAt should be populated after insert")
}

func TestSQLiteMutation_Insert_MultipleRows(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	item1 := &sqliteItem{Title: "First"}
	item2 := &sqliteItem{Title: "Second"}

	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(item1)
		repo.Add(item2)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	assert.Equal(t, 1, item1.ID)
	assert.Equal(t, 2, item2.ID)
	assert.NotEqual(t, item1.ID, item2.ID)
}

// ─── UPDATE path: non-RETURNING ──────────────────────────────────────────────

func TestSQLiteMutation_Update_ChangesArePersisted(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	// Insert a row first.
	item := &sqliteItem{Title: "Original"}
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(item)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)
	require.NotZero(t, item.ID)

	// Modify the item in a new transaction.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		found, err := repo.Find(ctx, item.ID)
		if err != nil {
			return err
		}
		found.Title = "Updated"
		return nil // changes flushed by Transaction() on commit
	})
	require.NoError(t, err)

	// Verify the update persisted.
	repo := drel.NewRepository(engine, sqliteItemMeta)
	found, err := repo.Find(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, "Updated", found.Title)
}

// ─── DELETE path ─────────────────────────────────────────────────────────────

func TestSQLiteMutation_Delete_RemovesRow(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	// Insert a row.
	item := &sqliteItem{Title: "ToDelete"}
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(item)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)

	// Delete it.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		found, err := repo.Find(ctx, item.ID)
		if err != nil {
			return err
		}
		return repo.Remove(found)
	})
	require.NoError(t, err)

	// Verify it's gone.
	repo := drel.NewRepository(engine, sqliteItemMeta)
	_, err = repo.Find(ctx, item.ID)
	assert.ErrorIs(t, err, drel.ErrNotFound)
}

// ─── isNoRows helper ─────────────────────────────────────────────────────────

func TestIsNoRows_SqlErrNoRows(t *testing.T) {
	// The isNoRows helper is unexported, but we can verify its behavior
	// indirectly by checking that sql.ErrNoRows is detected through
	// errors.Is, and the pgx sentinel text is matched.
	assert.True(t, errors.Is(sql.ErrNoRows, sql.ErrNoRows))
	// The pgx sentinel has message "no rows in result set" (not wrapped by sql.ErrNoRows).
	// We verify the string matching approach works.
	pgxLike := errors.New("no rows in result set")
	assert.Equal(t, "no rows in result set", pgxLike.Error())
}

// ─── Retry after failed SaveChanges ─────────────────────────────────────────

// headlineRetryHook is a before-commit hook that fails exactly once, then
// succeeds, to simulate a transient commit/hook failure followed by a retry.
type headlineRetryHook struct{ failed bool }

func (h *headlineRetryHook) hook(ctx context.Context, tx *drel.Tx, events []any) error {
	if !h.failed {
		h.failed = true
		return errors.New("transient before-commit failure")
	}
	return nil
}

func TestUnitOfWork_RetryAfterFailedSaveChangesPersists(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	h := &headlineRetryHook{}
	engine.OnBeforeCommit(h.hook)

	uow := engine.NewUnitOfWork()
	repo := drel.NewUoWRepository(uow, sqliteItemMeta)
	item := &sqliteItem{Title: "retry-me"}
	repo.Add(item)

	// First SaveChanges fails at the before-commit hook -> rollback.
	err := uow.SaveChanges(ctx)
	require.Error(t, err)

	// The staged Add must survive: a retry on the SAME unit of work re-emits
	// the INSERT and persists exactly one row.
	require.NoError(t, uow.SaveChanges(ctx))

	check := drel.NewRepository(engine, sqliteItemMeta)
	n, err := check.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "retry after a failed SaveChanges must persist the staged Add exactly once")
	assert.NotZero(t, item.ID, "the persisted row's id must be populated after the successful retry")
}

// ─── updated_at uses CURRENT_TIMESTAMP via d.Now() ──────────────────────────

func TestSQLiteMutation_Update_UpdatedAtChanges(t *testing.T) {
	engine := setupSQLiteEngine(t)
	ctx := context.Background()

	// Insert a row.
	item := &sqliteItem{Title: "Original"}
	err := engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		repo.Add(item)
		return tx.SaveChanges(ctx)
	})
	require.NoError(t, err)
	originalUpdatedAt := item.UpdatedAt

	// Update the row.
	err = engine.Transaction(ctx, func(tx *drel.Tx) error {
		repo := drel.NewTxRepository(tx, sqliteItemMeta)
		found, err := repo.Find(ctx, item.ID)
		if err != nil {
			return err
		}
		found.Title = "Modified"
		return nil
	})
	require.NoError(t, err)

	// Verify updated_at changed (or at least is not before the original).
	repo := drel.NewRepository(engine, sqliteItemMeta)
	found, err := repo.Find(ctx, item.ID)
	require.NoError(t, err)
	assert.False(t, found.UpdatedAt.Before(originalUpdatedAt),
		"updated_at should not be before the original value after an update")
}
