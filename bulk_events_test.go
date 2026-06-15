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
