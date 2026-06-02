package migrate_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel/internal/driver/sqlitedriver"
	"github.com/alternayte/drel/internal/migrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	runner := migrate.NewRunner(drv, dir)

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
