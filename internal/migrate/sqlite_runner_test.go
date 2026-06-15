package migrate_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/alternayte/drel/internal/migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunner_Lock_SQLite(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	_, err := migrate.WriteMigration(dir, "init",
		"CREATE TABLE a (id INTEGER PRIMARY KEY);",
		"DROP TABLE a;")
	require.NoError(t, err)

	drv, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	defer drv.Close()

	runner := migrate.NewRunner(drv, dir, "sqlite")

	// A normal Up acquires and releases the lock, so a second Up is fine.
	_, err = runner.Up(ctx)
	require.NoError(t, err)
	_, err = runner.Up(ctx)
	require.NoError(t, err)

	// Simulate a concurrent holder: insert the sentinel row directly, then Up
	// must report a clear "locked by another process" error.
	require.NoError(t, migrate.ForceLockForTest(ctx, drv))
	_, err = runner.Up(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "locked by another process")

	// Releasing the sentinel lets Up proceed again.
	require.NoError(t, migrate.ForceUnlockForTest(ctx, drv))
	_, err = runner.Up(ctx)
	require.NoError(t, err)
}

func TestRunner_Pending(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	_, err := migrate.WriteMigration(dir, "init",
		"CREATE TABLE a (id INTEGER PRIMARY KEY);", "DROP TABLE a;")
	require.NoError(t, err)
	_, err = migrate.WriteMigration(dir, "add_b",
		"CREATE TABLE b (id INTEGER PRIMARY KEY);", "DROP TABLE b;")
	require.NoError(t, err)

	drv, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	defer drv.Close()

	runner := migrate.NewRunner(drv, dir, "sqlite")

	// Before any Up, both migrations are pending.
	pending, err := runner.Pending(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	assert.Equal(t, "init", pending[0].Name)
	assert.Equal(t, "add_b", pending[1].Name)

	// After Up, none are pending.
	_, err = runner.Up(ctx)
	require.NoError(t, err)
	pending, err = runner.Pending(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

// TestRunner_SQLite_EndToEnd exercises the migration runner against the SQLite
// driver: a portable tracking table, multi-statement Up SQL, Status, Lint, and
// Down all working without any Postgres-specific syntax.
func TestRunner_SQLite_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// First migration: multi-statement CREATE.
	_, err := migrate.WriteMigration(dir, "init",
		"CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT NOT NULL);\nCREATE TABLE gadgets (id INTEGER PRIMARY KEY);",
		"DROP TABLE gadgets;\nDROP TABLE widgets;")
	require.NoError(t, err)
	// Second migration: ALTER.
	_, err = migrate.WriteMigration(dir, "add_color",
		"ALTER TABLE widgets ADD COLUMN color TEXT;",
		"ALTER TABLE widgets DROP COLUMN color;")
	require.NoError(t, err)

	drv, err := sqlitedriver.New(":memory:")
	require.NoError(t, err)
	defer drv.Close()

	runner := migrate.NewRunner(drv, dir, "sqlite")

	n, err := runner.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	// The schema is real: inserting into the altered table works.
	_, err = drv.Exec(ctx, "INSERT INTO widgets (name, color) VALUES ('w', 'red')")
	require.NoError(t, err)

	// Idempotent: a second Up applies nothing.
	n, err = runner.Up(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// Status reports both as applied.
	statuses, err := runner.Status(ctx)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	for _, s := range statuses {
		assert.Equal(t, migrate.StateApplied, s.State)
	}

	// Lint passes (checksums intact).
	issues, err := runner.Lint(ctx)
	require.NoError(t, err)
	assert.Empty(t, issues)

	// Down rolls back the most recent migration.
	require.NoError(t, runner.Down(ctx))
	_, err = drv.Exec(ctx, "INSERT INTO widgets (name, color) VALUES ('x', 'blue')")
	assert.Error(t, err, "color column should be gone after rollback")
}
