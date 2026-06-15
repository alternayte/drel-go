package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBulkInsert_DropsEvents documents that plain BulkInsert bypasses change
// tracking, so RecordEvent events are NOT written to the outbox.
func TestBulkInsert_DropsEvents(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()
	ctx := context.Background()

	_, err = engine.Exec(ctx, `CREATE TABLE ev_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, drel.OutboxSchema("outbox", "sqlite"))
	require.NoError(t, err)
	engine.UseOutbox("outbox")

	repo := drel.NewRepository(engine, evItemMeta)
	n, err := repo.BulkInsert(ctx, []*evItem{
		{Name: "a", events: []any{itemCreated{Name: "a"}}},
		{Name: "b", events: []any{itemCreated{Name: "b"}}},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// Documented behaviour: no outbox rows — bulk bypasses tracking/events.
	var c int
	require.NoError(t, engine.QueryRow(ctx, `SELECT COUNT(*) FROM outbox`).Scan(&c))
	assert.Equal(t, 0, c, "plain BulkInsert must NOT dispatch events (documented)")
}

// TestBulkInsertWithEvents_PersistsAndDispatches proves the opt-in variant
// collects EventRecorder events, writes them to the outbox in the same tx, and
// dispatches them after commit.
func TestBulkInsertWithEvents_PersistsAndDispatches(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()
	ctx := context.Background()

	_, err = engine.Exec(ctx, `CREATE TABLE ev_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = engine.Exec(ctx, drel.OutboxSchema("outbox", "sqlite"))
	require.NoError(t, err)
	engine.UseOutbox("outbox")

	var dispatched int
	engine.OnAfterCommit(func(ctx context.Context, events []any) { dispatched += len(events) })

	repo := drel.NewRepository(engine, evItemMeta)
	n, err := repo.BulkInsertWithEvents(ctx, []*evItem{
		{Name: "a", events: []any{itemCreated{Name: "a"}}},
		{Name: "b", events: []any{itemCreated{Name: "b"}}},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// Outbox rows committed atomically with the bulk insert.
	var c int
	require.NoError(t, engine.QueryRow(ctx, `SELECT COUNT(*) FROM outbox`).Scan(&c))
	assert.Equal(t, 2, c, "events must be persisted to the outbox")

	// And dispatched after commit.
	assert.Equal(t, 2, dispatched)
}

// TestBulkInsertWithEvents_RollsBackOnHookError proves an outbox/hook failure
// rolls back the whole bulk insert.
func TestBulkInsertWithEvents_RollsBackOnHookError(t *testing.T) {
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	defer engine.Close()
	ctx := context.Background()

	_, err = engine.Exec(ctx, `CREATE TABLE ev_items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	require.NoError(t, err)

	// A before-commit hook that always errors forces a rollback.
	engine.OnBeforeCommit(func(ctx context.Context, tx *drel.Tx, events []any) error {
		if len(events) > 0 {
			return assert.AnError
		}
		return nil
	})

	repo := drel.NewRepository(engine, evItemMeta)
	_, err = repo.BulkInsertWithEvents(ctx, []*evItem{
		{Name: "a", events: []any{itemCreated{Name: "a"}}},
	})
	require.Error(t, err)

	// The row must NOT be present — the bulk insert rolled back.
	var c int
	require.NoError(t, engine.QueryRow(ctx, `SELECT COUNT(*) FROM ev_items`).Scan(&c))
	assert.Equal(t, 0, c, "bulk insert must roll back when an event hook errors")
}
