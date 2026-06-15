package drel_test

import (
	"context"
	"testing"

	"github.com/alternayte/drel"
	"github.com/stretchr/testify/require"
)

type widgetRow struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
	Kind string `db:"kind"`
}

func setupPredicateRuntimeEngine(t *testing.T) *drel.Engine {
	t.Helper()
	engine, err := drel.NewEngine(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { engine.Close() })

	ctx := context.Background()
	_, err = engine.Exec(ctx, `
		CREATE TABLE widgets (
			id   INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			kind TEXT NOT NULL
		)
	`)
	require.NoError(t, err)
	for _, w := range []struct{ name, kind string }{
		{"a", "alpha"}, {"b", "beta"}, {"c", "alpha"},
	} {
		_, err = engine.Exec(ctx, "INSERT INTO widgets (name, kind) VALUES (?, ?)", w.name, w.kind)
		require.NoError(t, err)
	}
	return engine
}

// Empty IN must produce valid SQL that matches zero rows; empty NOT IN must
// match every row. These mirror the emitter contract (0 / 1 for SQLite) so the
// generated fragment actually executes.
func TestRuntime_EmptyIn_Executes(t *testing.T) {
	engine := setupPredicateRuntimeEngine(t)
	ctx := context.Background()

	// SELECT ... WHERE kind IN ()  ->  emitter renders "0" -> zero rows.
	in, err := drel.RawQuery[widgetRow](ctx, engine,
		"SELECT id, name, kind FROM widgets WHERE 0")
	require.NoError(t, err)
	require.Len(t, in, 0)

	// SELECT ... WHERE kind NOT IN ()  ->  emitter renders "1" -> all rows.
	notIn, err := drel.RawQuery[widgetRow](ctx, engine,
		"SELECT id, name, kind FROM widgets WHERE 1")
	require.NoError(t, err)
	require.Len(t, notIn, 3)
}
